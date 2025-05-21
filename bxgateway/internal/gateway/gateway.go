package gateway

import (
	"context"
	"fmt"
	"net/netip"
	"sync"
	"syscall"
	"time"

	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/netlisten"
	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/txfwd"
	"github.com/bloXroute-Labs/solana-gateway/pkg/cache"
	"github.com/bloXroute-Labs/solana-gateway/pkg/jwt"
	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/ofr"
	"github.com/bloXroute-Labs/solana-gateway/pkg/solana"
	"github.com/bloXroute-Labs/solana-gateway/pkg/udp"
)

const (
	aliveMsgInterval = 10 * time.Second

	// noTrafficThreshold defines a period within which OFR is suppose to provide traffic,
	// if no traffic is received then the gateway makes additional Register call
	noTrafficThreshold = time.Second * 10
)

type Gateway struct {
	ctx                       context.Context
	lg                        logger.Logger
	cache                     *cache.AlterKey
	stats                     *ofr.Stats
	nl                        *netlisten.NetworkListener
	serverFd                  *udp.FDConn // fd receiving traffic from ofr
	send2OFRFd                *udp.FDConn // fd sending traffic to ofr
	send2NodeFd               *udp.FDConn // fd sending traffic to node
	solanaTVUAddr             syscall.Sockaddr
	ofrUDPAddrMx              *sync.RWMutex
	ofrUDPAddr                udp.Addr
	registrar                 Registrar
	noTrafficTicker           *time.Ticker
	refreshJWTTicker          *time.Ticker
	bxForwarder               *txfwd.BxForwarder
	traderAPIFwd              *txfwd.TraderAPIForwarder
	extraBroadcastAddrs       []syscall.Sockaddr // additional endpoints set via --broadcast-addresses
	extraBroadcastFromOFROnly bool
	noValidator               bool
	passiveMode               bool
	jwt                       bool
}

type shredPacket struct {
	payload [solana.UDPShredSize]byte
	length  int
}

type Option func(*Gateway)

func PassiveMode() Option { return func(g *Gateway) { g.passiveMode = true } }

func WithoutSolanaNode() Option { return func(g *Gateway) { g.noValidator = true } }

func WithBroadcastAddrs(addrs []string, ofrOnly bool) (Option, error) {
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
		g.extraBroadcastFromOFROnly = ofrOnly
	}, nil
}

func WithTxForwarders(bxForwarders *txfwd.BxForwarder, traderAPIFwd *txfwd.TraderAPIForwarder) Option {
	return func(g *Gateway) {
		g.jwt = true
		g.bxForwarder = bxForwarders
		g.traderAPIFwd = traderAPIFwd
	}
}

func New(
	ctx context.Context,
	lg logger.Logger,
	cache *cache.AlterKey,
	stats *ofr.Stats,
	nl *netlisten.NetworkListener,
	serverFd *udp.FDConn,
	fdset *udp.FDSet,
	solanaTVUAddr syscall.Sockaddr,
	registrar Registrar,
	opts ...Option,
) (*Gateway, error) {
	snd2OFRFd, err := fdset.NextFD()
	if err != nil {
		return nil, fmt.Errorf("new fd: %s", err)
	}

	snd2NodeFd, err := fdset.NextFD()
	if err != nil {
		return nil, fmt.Errorf("new fd: %s", err)
	}

	gw := &Gateway{
		ctx:              ctx,
		lg:               lg,
		cache:            cache,
		stats:            stats,
		nl:               nl,
		serverFd:         serverFd,
		send2OFRFd:       snd2OFRFd,
		send2NodeFd:      snd2NodeFd,
		solanaTVUAddr:    solanaTVUAddr,
		ofrUDPAddrMx:     &sync.RWMutex{},
		ofrUDPAddr:       udp.Addr{}, // this addr is returned from registration endpoint
		passiveMode:      false,
		registrar:        registrar,
		refreshJWTTicker: stoppedTicker(), // will be reset if needed during registration
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

	localAddrString := g.nl.Addr().String()

	for unseenShreds := range g.stats.RecvFlushUnseenShreds() {
		var totalShreds, totalShredsSeenFromOFR int

		for _, st := range unseenShreds.Stats {
			totalShreds += st.Shreds
			if st.Src != localAddrString {
				totalShredsSeenFromOFR += st.Shreds
			}
		}

		if totalShreds == 0 {
			g.lg.Infof("No shreds received")
			continue
		}

		g.lg.Infof("Seen %.2f%% of shreds first from OFR", (float64(totalShredsSeenFromOFR)/float64(totalShreds))*100)
	}
}

// Register calls register endpoint and sets returned ofrUDPAddr
func (g *Gateway) Register() (syscall.Sockaddr, error) {
	// unregister the previous address
	g.ofrUDPAddrMx.Lock()
	g.ofrUDPAddr = udp.Addr{}
	g.ofrUDPAddrMx.Unlock()

	rsp, err := g.registrar.Register()
	if err != nil {
		return nil, fmt.Errorf("register gateway: %s", err)
	}

	sockAddr, err := udp.SockAddrFromUDPString(rsp.GetUdpAddress())
	if err != nil {
		return nil, fmt.Errorf("resolve OFR UDP address: %s", err)
	}

	addr, err := udp.NewAddr(sockAddr)
	if err != nil {
		return nil, fmt.Errorf("parse sockaddr: %s %s", udp.SockaddrString(sockAddr), err)
	}

	g.ofrUDPAddrMx.Lock()
	g.ofrUDPAddr = addr
	g.ofrUDPAddrMx.Unlock()

	jwtToken := rsp.GetJwtToken()

	if g.bxForwarder != nil {
		go g.bxForwarder.Update(jwtToken, rsp.GetTxPropagationConfig())
	}

	if g.traderAPIFwd != nil {
		go g.traderAPIFwd.Update(rsp.GetTxPropagationConfig())
	}

	if jwtToken != "" {
		err = g.resetRefreshJWTTicker(jwtToken)
		if err != nil {
			g.lg.Errorf("failed to reset refresh JWT ticker: %s", err)
		}
	}

	return sockAddr, nil
}

func (g *Gateway) Start() {
	addr, err := g.Register()
	if err != nil {
		g.lg.Errorf("register gateway on startup: %s", err)

		time.Sleep(time.Minute)

		return
	}

	g.lg.Infof("gateway successfully registered, udp addr: %s", udp.SockaddrString(addr))

	go g.reRegister()

	ofrNodeCh := make(chan shredData, 1e3)
	go g.receiveShredsFromOFR(ofrNodeCh)
	go g.broadcastShredsToNode(ofrNodeCh)

	if g.noValidator {
		go g.sendAliveMessages()
	} else {
		node2OFRCh := make(chan netlisten.Shred, 1e3)
		go g.nl.Recv(node2OFRCh)
		go g.broadcastShredsToOFR(node2OFRCh)
	}
}

func (g *Gateway) sendAliveMessages() {
	ticker := time.NewTicker(aliveMsgInterval)
	defer ticker.Stop()

	for {
		if !g.ofrUDPAddr.IsZero() {
			if err := g.send2OFRFd.UnsafeWrite(solana.AliveMsg, g.ofrUDPAddr.SockAddr); err != nil {
				g.lg.Errorf("sendAliveMessages: write to UDP: %v", err)
			}
		}

		<-ticker.C
	}
}

type shredData struct {
	packet shredPacket
	time   time.Time

	shred solana.PartialShred
	src   netip.Addr
}

func (g *Gateway) processShred(i int, broadcastCh chan shredData) (packet shredPacket) {
	n, addr, err := g.serverFd.UnsafeReadFrom(packet.payload[:])
	if err != nil {
		g.lg.Errorf("read from udp: %s", err)
		return
	}
	packet.length = n

	if g.ofrUDPAddr.IsZero() {
		g.lg.Warnf("ofrUDPAddr is empty")
		return
	}

	// only check if IPs are equal due to Relays use different ports to send are recv traffic
	if addr.NetipAddr != g.ofrUDPAddr.NetipAddr {
		return
	}

	g.noTrafficTicker.Reset(noTrafficThreshold)
	if g.passiveMode {
		return
	}

	now := time.Now()
	shred, err := solana.ParseShredPartial(packet.payload)
	if err != nil {
		g.lg.Errorf("ofr: failed to analyze packet from ofr: %s", err)
		return
	}

	g.lg.Tracef("gateway: recv shred, slot: %d, index: %d, from: %s", shred.Slot, shred.Index, addr.NetipAddr.String())
	if i == 1e5 {
		g.lg.Debugf("health: receiveShredsFromOFR 100K: broadcast buf: %d", len(broadcastCh))
		i = 0
	}

	g.stats.RecordNewShred(addr.NetipAddr, "OFR")

	if !g.cache.Set(solana.ShredKey(shred.Slot, shred.Index, shred.Variant)) {
		return
	}

	g.stats.RecordUnseenShred(addr.NetipAddr, shred, "OFR")

	select {
	case broadcastCh <- shredData{
		packet: packet,
		time:   now,
		shred:  shred,
		src:    addr.NetipAddr,
	}:
	default:
		g.lg.Warn("gateway: forward shred from ofr: channel is full")
	}
	return
}

func (g *Gateway) receiveShredsFromOFR(broadcastCh chan shredData) {
	for i := 0; ; i++ {
		select {
		case <-g.ctx.Done():
			return
		default:
		}

		g.processShred(i, broadcastCh)
	}
}

func (g *Gateway) broadcastShredsToNode(ch <-chan shredData) {
	if g.passiveMode {
		return
	}

	for shred := range ch {
		if !g.noValidator {
			if err := g.send2NodeFd.UnsafeWrite(shred.packet.payload[:shred.packet.length], g.solanaTVUAddr); err != nil {
				g.lg.Errorf("broadcast to solana node: write to UDP: %s: %s", udp.SockaddrString(g.solanaTVUAddr), err)
			}
		}

		for _, addr := range g.extraBroadcastAddrs {
			if err := g.send2NodeFd.UnsafeWrite(shred.packet.payload[:shred.packet.length], addr); err != nil {
				g.lg.Errorf("broadcast to extra addr: write to UDP: %s: %s", udp.SockaddrString(addr), err)
			}
		}

		if solana.ShouldSample(shred.shred) {
			g.stats.RecordShredGateway(shred.shred.Slot, shred.shred.Index, shred.shred.Variant.String(), shred.src.String(), time.Now(), time.Since(shred.time))
		}
	}
}

func (g *Gateway) broadcastShredsToOFR(ch <-chan netlisten.Shred) {
	for shred := range ch {
		g.ofrUDPAddrMx.RLock()
		ofrUDPAddr := g.ofrUDPAddr
		g.ofrUDPAddrMx.RUnlock()

		if ofrUDPAddr.IsZero() {
			continue // we are not yet registered
		}

		err := g.send2OFRFd.UnsafeWrite(shred.Packet.Payload[:shred.Packet.Length], ofrUDPAddr.SockAddr)
		if err != nil {
			g.lg.Errorf("broadcast to OFR: write to UDP addr: %s: %s", udp.SockaddrString(ofrUDPAddr.SockAddr), err)
		}

		if !g.extraBroadcastFromOFROnly {
			for _, addr := range g.extraBroadcastAddrs {
				err = g.send2OFRFd.UnsafeWrite(shred.Packet.Payload[:shred.Packet.Length], addr)
				if err != nil {
					g.lg.Errorf("broadcast to extra addr: write to UDP: %s: %s", udp.SockaddrString(addr), err)
				}
			}
		}

		if solana.ShouldSample(shred.Shred) {
			g.stats.RecordShredGateway(shred.Shred.Slot, shred.Shred.Index, shred.Shred.Variant.String(), shred.Packet.SrcAddr.String(), shred.Packet.ReceiveTime, time.Since(shred.Packet.ReceiveTime))
		}
	}
}

func (g *Gateway) reRegister() {
	wait := noTrafficThreshold

	// start monitoring if we need to register again
	g.noTrafficTicker = time.NewTicker(noTrafficThreshold)

	for {
		select {
		case <-g.ctx.Done():
			return
		case <-g.noTrafficTicker.C:
			g.lg.Infof("no traffic from OFR: re-registering gateway...")

			addr, err := g.Register()
			if err != nil {
				g.lg.Errorf("no traffic from OFR: re-register: %s", err)

				if wait < time.Hour {
					wait = 2 * wait
				}

				g.noTrafficTicker.Reset(wait)

				continue
			}

			wait = noTrafficThreshold

			// reset the ticker after the callback is called
			// to avoid calling the callback multiple times
			// if the callback takes longer than the ticker duration
			g.noTrafficTicker.Reset(noTrafficThreshold)

			g.lg.Infof("no traffic from OFR: gateway successfully re-registered, udp addr: %s", udp.SockaddrString(addr))
		case <-g.refreshJWTTicker.C:
			g.lg.Infof("refreshing JWT Token")
			rsp, err := g.registrar.RefreshToken(g.bxForwarder.Token())
			if err != nil {
				g.lg.Errorf("failed to refresh token: %s, trying to re-register", err)

				addr, err := g.Register()
				if err != nil {
					g.lg.Errorf("failed to re-register due to token refresh error: %s", err)
					continue
				}

				g.noTrafficTicker.Reset(noTrafficThreshold)
				g.lg.Infof("gateway successfully re-registered, udp addr: %s", udp.SockaddrString(addr))
				continue
			}

			g.bxForwarder.UpdateToken(rsp.GetJwtToken())
			err = g.resetRefreshJWTTicker(rsp.GetJwtToken())
			if err != nil {
				g.lg.Errorf("failed to reset refresh JWT ticker: %s", err)
			}
		}
	}
}

func (g *Gateway) resetRefreshJWTTicker(jwtToken string) error {
	if !g.jwt { // no need to reset ticker if JWT is not used
		return nil
	}

	token, err := jwt.ParseJWT(jwtToken)
	if err != nil {
		g.lg.Warnf("failed to parse JWT: %s %s", jwtToken, err)
		return fmt.Errorf("failed to parse JWT: %s", err)
	}

	exp, err := jwt.GetExpirationTimeFromJWT(token)
	if err != nil {
		return fmt.Errorf("failed to get expiration time from JWT: %s", err)
	}

	g.refreshJWTTicker.Reset(time.Until(exp) - time.Minute)
	return nil
}

// stoppedTicker returns a ticker that is stopped immediately, but can be reset if/when needed
func stoppedTicker() *time.Ticker {
	ticker := time.NewTicker(time.Second)
	ticker.Stop()
	return ticker
}
