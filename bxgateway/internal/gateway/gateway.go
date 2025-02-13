package gateway

import (
	"context"
	"fmt"
	"sync"
	"syscall"
	"time"

	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/netlisten"
	"github.com/bloXroute-Labs/solana-gateway/pkg/bdn"
	"github.com/bloXroute-Labs/solana-gateway/pkg/cache"
	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/packet"
	"github.com/bloXroute-Labs/solana-gateway/pkg/solana"
	"github.com/bloXroute-Labs/solana-gateway/pkg/udp"
)

const (
	udpShredSize     = 1228
	aliveMsgInterval = 10 * time.Second

	// noTrafficThreshold defines a period within which BDN is suppose to provide traffic,
	// if no traffic is received then the gateway makes additional Register call
	noTrafficThreshold = time.Second * 10
)

type Gateway struct {
	ctx           context.Context
	lg            logger.Logger
	cache         *cache.AlterKey
	stats         *bdn.Stats
	nl            *netlisten.NetworkListener
	pool          *sync.Pool
	serverFd      *udp.FDConn // fd receiving traffic from bdn
	send2BDNFd    *udp.FDConn // fd sending traffic to bdn
	send2NodeFd   *udp.FDConn // fd sending traffic to node
	solanaTVUAddr syscall.Sockaddr
	bdnUDPAddrMx  *sync.RWMutex
	bdnUDPAddr    *udp.Addr
	registrar     Registrar
	noTraffic     *time.Ticker

	extraBroadcastAddrs       []syscall.Sockaddr // additional endpoints set via --broadcast-addresses
	extraBroadcastFromBDNOnly bool
	noValidator               bool
	passiveMode               bool
}

type Option func(*Gateway)

func PassiveMode() Option { return func(g *Gateway) { g.passiveMode = true } }

func WithoutSolanaNode() Option { return func(g *Gateway) { g.noValidator = true } }

func WithBroadcastAddrs(addrs []string, bdnOnly bool) (Option, error) {
	var sockaddrs []syscall.Sockaddr

	for _, addr := range addrs {
		sockaddr, err := udp.SockAddrFromUDPString(addr)
		if err != nil {
			return nil, err
		}

		sockaddrs = append(sockaddrs, sockaddr)
	}

	return func(g *Gateway) {
		g.extraBroadcastAddrs = sockaddrs
		g.extraBroadcastFromBDNOnly = bdnOnly
	}, nil
}

func New(
	ctx context.Context,
	lg logger.Logger,
	cache *cache.AlterKey,
	stats *bdn.Stats,
	nl *netlisten.NetworkListener,
	serverFd *udp.FDConn,
	fdset *udp.FDSet,
	solanaTVUAddr syscall.Sockaddr,
	registrar Registrar,
	opts ...Option,
) (*Gateway, error) {
	snd2BDNFd, err := fdset.NextFD()
	if err != nil {
		return nil, fmt.Errorf("new fd: %s", err)
	}

	snd2NodeFd, err := fdset.NextFD()
	if err != nil {
		return nil, fmt.Errorf("new fd: %s", err)
	}

	gw := &Gateway{
		ctx:           ctx,
		lg:            lg,
		cache:         cache,
		stats:         stats,
		nl:            nl,
		pool:          &sync.Pool{New: func() interface{} { s := make([]byte, udpShredSize); return &s }},
		serverFd:      serverFd,
		send2BDNFd:    snd2BDNFd,
		send2NodeFd:   snd2NodeFd,
		solanaTVUAddr: solanaTVUAddr,
		bdnUDPAddrMx:  &sync.RWMutex{},
		bdnUDPAddr:    nil, // this addr is returned from registration endpoint
		passiveMode:   false,
		registrar:     registrar,
	}

	for _, o := range opts {
		o(gw)
	}

	go gw.printUnseenStats()
	return gw, nil
}

func (g *Gateway) printUnseenStats() {
	if g.noValidator {
		return
	}

	var localAddrString = g.nl.Addr().String()

	for unseenShreds := range g.stats.RecvFlushUnseenShreds() {
		var totalShreds, totalShredsSeenFromBdn int

		for _, st := range unseenShreds.Stats {
			totalShreds += st.Shreds
			if st.Src != localAddrString {
				totalShredsSeenFromBdn += st.Shreds
			}
		}

		if totalShreds == 0 {
			g.lg.Infof("No shreds received")
		}

		g.lg.Infof("Seen %.2f%% of shreds first from BDN", (float64(totalShredsSeenFromBdn)/float64(totalShreds))*100)
	}
}

// Register calls register endpoint and sets returned bdnUDPAddr
func (g *Gateway) Register() (syscall.Sockaddr, error) {
	// unregister the previous address
	g.bdnUDPAddrMx.Lock()
	g.bdnUDPAddr = nil
	g.bdnUDPAddrMx.Unlock()

	udpAddr, err := g.registrar.Register()
	if err != nil {
		return nil, fmt.Errorf("register gateway: %s", err)
	}

	sockAddr, err := udp.SockAddrFromUDPString(udpAddr)
	if err != nil {
		return nil, fmt.Errorf("resolve BDN UDP address: %s", err)
	}

	addr, err := udp.NewAddr(sockAddr)
	if err != nil {
		return nil, fmt.Errorf("parse sockaddr: %s %s", udp.SockaddrString(sockAddr), err)
	}

	g.bdnUDPAddrMx.Lock()
	defer g.bdnUDPAddrMx.Unlock()

	g.bdnUDPAddr = addr

	return sockAddr, nil
}

func (g *Gateway) Start() {
	var (
		bdn2NodeCh = make(chan *[]byte, 1e5)
		node2BDNCh = make(chan *packet.SniffPacket, 1e5)
	)

	addr, err := g.Register()
	if err != nil {
		time.Sleep(time.Minute)

		g.lg.Errorf("register gateway on startup: %s", err)

		return
	}

	g.lg.Infof("gateway successfully registered, udp addr: %s", udp.SockaddrString(addr))

	go g.reRegister()
	go g.receiveShredsFromBDN(bdn2NodeCh)
	go g.broadcastShredsToNode(bdn2NodeCh)

	if g.noValidator {
		go g.sendAliveMessages()
	} else {
		go g.nl.Recv(node2BDNCh)
		go g.broadcastShredsToBDN(node2BDNCh)
	}
}

func (g *Gateway) sendAliveMessages() {
	ticker := time.NewTicker(aliveMsgInterval)
	defer ticker.Stop()

	for {
		if err := g.send2BDNFd.UnsafeWrite([]byte(solana.AliveMsg), g.bdnUDPAddr.SockAddr); err != nil {
			g.lg.Errorf("sendAliveMessages: write to UDP: %v", err)
		}

		<-ticker.C
	}
}

func (g *Gateway) receiveShredsFromBDN(broadcastCh chan *[]byte) {
	var done = g.ctx.Done()

	for i := 0; ; i++ {
		select {
		case <-done:
			return
		default:
		}

		buf := g.pool.Get().(*[]byte)
		_, addr, err := g.serverFd.UnsafeReadFrom(*buf)
		if err != nil {
			g.lg.Errorf("read from udp: %s", err)
			g.pool.Put(buf)
			continue
		}

		if g.bdnUDPAddr == nil {
			g.lg.Warnf("bdnUDPAddr is nil")
			g.pool.Put(buf)
			continue
		}

		// only check if IPs are equal due to Relays use different ports to send are recv traffic
		if addr.NetipAddr != g.bdnUDPAddr.NetipAddr {
			g.pool.Put(buf)
			continue
		}

		g.noTraffic.Reset(noTrafficThreshold)

		shred, err := solana.ParseShredPartial(*buf)
		if err != nil {
			g.lg.Errorf("bdn: failed to analyze packet from bdn: %s", err)
			g.pool.Put(buf)
			continue
		}

		g.lg.Tracef("gateway: recv shred, slot: %d, index: %d, from: %s", shred.Slot, shred.Index, addr)
		if i == 1e5 {
			g.lg.Debugf("health: receiveShredsFromBDN 100K: broadcast buf: %d", len(broadcastCh))
			i = 0
		}

		g.stats.RecordNewShred(addr.NetipAddr)

		if !g.cache.Set(solana.ShredKey(shred.Slot, shred.Index, shred.Variant)) {
			g.pool.Put(buf)
			continue
		}

		g.stats.RecordUnseenShred(addr.NetipAddr, shred)

		if g.passiveMode {
			g.pool.Put(buf)
			continue
		}

		select {
		case broadcastCh <- buf:
		default:
			g.pool.Put(buf)
			g.lg.Warn("gateway: forward shred from bdn: channel is full")
		}
	}
}

func (g *Gateway) broadcastShredsToNode(ch <-chan *[]byte) {
	if g.passiveMode {
		return
	}

	for buf := range ch {
		if !g.noValidator {
			if err := g.send2NodeFd.UnsafeWrite(*buf, g.solanaTVUAddr); err != nil {
				g.lg.Errorf("broadcast to solana node: write to UDP: %s: %s", udp.SockaddrString(g.solanaTVUAddr), err)
			}
		}

		for _, addr := range g.extraBroadcastAddrs {
			if err := g.send2NodeFd.UnsafeWrite(*buf, addr); err != nil {
				g.lg.Errorf("broadcast to extra addr: write to UDP: %s: %s", udp.SockaddrString(addr), err)
			}
		}

		g.pool.Put(buf)
	}
}

func (g *Gateway) broadcastShredsToBDN(ch <-chan *packet.SniffPacket) {
	for pkt := range ch {
		g.bdnUDPAddrMx.RLock()
		bdnUDPAddr := g.bdnUDPAddr
		g.bdnUDPAddrMx.RUnlock()

		if bdnUDPAddr == nil {
			pkt.Free()
			continue // we are not yet registered
		}

		err := g.send2BDNFd.UnsafeWrite(pkt.Payload, g.bdnUDPAddr.SockAddr)
		if err != nil {
			g.lg.Errorf("broadcast to BDN: write to UDP addr: %s: %s", udp.SockaddrString(g.bdnUDPAddr.SockAddr), err)
		}

		if !g.extraBroadcastFromBDNOnly {
			for _, addr := range g.extraBroadcastAddrs {
				err = g.send2BDNFd.UnsafeWrite(pkt.Payload, addr)
				if err != nil {
					g.lg.Errorf("broadcast to extra addr: write to UDP: %s: %s", udp.SockaddrString(addr), err)
				}
			}
		}

		pkt.Free()
	}
}

func (g *Gateway) reRegister() {
	wait := noTrafficThreshold

	// start monitoring if we need to register again
	g.noTraffic = time.NewTicker(noTrafficThreshold)

	for {
		select {
		case <-g.ctx.Done():
			return
		case <-g.noTraffic.C:
			g.lg.Infof("no traffic from BDN: re-registering gateway...")

			addr, err := g.Register()
			if err != nil {
				g.lg.Errorf("no traffic from BDN: re-register: %s", err)

				if wait < time.Hour {
					wait = 2 * wait
				}

				g.noTraffic.Reset(wait)

				continue
			}

			wait = noTrafficThreshold

			// reset the ticker after the callback is called
			// to avoid calling the callback multiple times
			// if the callback takes longer than the ticker duration
			g.noTraffic.Reset(noTrafficThreshold)

			g.lg.Infof("no traffic from BDN: gateway successfully re-registered, udp addr: %s", udp.SockaddrString(addr))
		}
	}
}
