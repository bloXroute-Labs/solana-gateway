package gateway

import (
	"context"
	"fmt"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/bloXroute-Labs/solana-gateway/bxgateway/internal/netlisten"
	"github.com/bloXroute-Labs/solana-gateway/pkg/bdn"
	"github.com/bloXroute-Labs/solana-gateway/pkg/cache"
	inactivitytrigger "github.com/bloXroute-Labs/solana-gateway/pkg/inactivity_trigger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/bloXroute-Labs/solana-gateway/pkg/solana"
)

const (
	udpShredSize     = 1228
	aliveMsgInterval = 10 * time.Second
)

type Gateway struct {
	ctx                          context.Context
	lg                           logger.Logger
	cache                        *cache.AlterKey
	stats                        *bdn.Stats
	nl                           *netlisten.NetworkListener
	pool                         *sync.Pool
	conn                         *net.UDPConn
	solana                       *net.UDPAddr
	bdnUDPAddrMx                 *sync.RWMutex
	bdnUDPAddr                   *net.UDPAddr
	registrar                    Registrar
	bdnRegisterInactivityTrigger *inactivitytrigger.InactivityTrigger
	addressesToSendShreds        []*net.UDPAddr
	broadcastFromBdnOnly         bool
	noValidator                  bool

	passiveMode bool
}

type Option func(*Gateway)

func PassiveMode() Option { return func(g *Gateway) { g.passiveMode = true } }

func parseCmdAddresses(addresses []string) []*net.UDPAddr {
	var addressesList []*net.UDPAddr
	if len(addresses) == 0 {
		return addressesList
	}
	for _, address := range addresses {
		if address == "" {
			panic(fmt.Errorf("argument to --broadcast-addresses is empty or has an extra comma"))
		}
		udpAddr, err := net.ResolveUDPAddr("udp", address)
		if err != nil {
			panic(fmt.Sprintf("failed to resolve address %v: %v", address, err))
		}
		addressesList = append(addressesList, udpAddr)
	}
	return addressesList
}

func New(ctx context.Context, lg logger.Logger, cache *cache.AlterKey, conn *net.UDPConn, solana *net.UDPAddr, stats *bdn.Stats, nl *netlisten.NetworkListener,
	registrar Registrar, addressesToSendShreds []string, broadcastFromBdnOnly bool, noValidator bool, opts ...Option) (*Gateway, error) {
	gw := &Gateway{
		ctx:    ctx,
		lg:     lg,
		cache:  cache,
		stats:  stats,
		nl:     nl,
		pool:   &sync.Pool{New: func() interface{} { return make([]byte, udpShredSize) }},
		conn:   conn,
		solana: solana,

		bdnUDPAddrMx:          &sync.RWMutex{},
		bdnUDPAddr:            nil,
		passiveMode:           false,
		registrar:             registrar,
		addressesToSendShreds: parseCmdAddresses(addressesToSendShreds),
		broadcastFromBdnOnly:  broadcastFromBdnOnly,
		noValidator:           noValidator,
	}

	gw.bdnRegisterInactivityTrigger = inactivitytrigger.NewInactivityTrigger(ctx, func() {
		udpAddr, err := registrar.Register()
		if err != nil {
			lg.Errorf("failed to register gateway after inactivity trigger, reason: %s", err)
		}

		bdnUDPAddr, err := net.ResolveUDPAddr("udp", udpAddr)
		if err != nil {
			lg.Errorf("resolve BDN UDP address: %s", err)
		}

		gw.bdnUDPAddrMx.Lock()
		gw.bdnUDPAddr = bdnUDPAddr
		gw.bdnUDPAddrMx.Unlock()

		lg.Infof("gateway successfully re-registered, udp addr: %s", bdnUDPAddr.String())
	}, time.Minute)

	for _, o := range opts {
		o(gw)
	}

	return gw, nil
}

func (g *Gateway) Start() {
	var (
		bdn2solCh = make(chan []byte, 1e5)
		sol2bdnCh = make(chan *solana.PartialShred, 1e5)
	)

	udpAddr, err := g.registrar.Register()
	if err != nil {
		g.lg.Errorf("register gateway on startup: %s", err)
		return
	}

	bdnUDPAddr, err := net.ResolveUDPAddr("udp", udpAddr)
	if err != nil {
		g.lg.Errorf("resolve BDN UDP address on startup: %s", err)
	}

	g.bdnUDPAddrMx.Lock()
	g.bdnUDPAddr = bdnUDPAddr
	g.bdnUDPAddrMx.Unlock()

	g.bdnRegisterInactivityTrigger.Start()

	g.lg.Infof("gateway successfully registered, udp addr: %s", bdnUDPAddr.String())

	for i := 0; i < runtime.NumCPU(); i++ {
		go g.receiveShredsFromBDN(bdn2solCh)
	}

	for i := 0; i < runtime.NumCPU()/2; i++ {
		go g.broadcastToSolana(bdn2solCh)
		if !g.noValidator {
			go g.broadcastToBDN(sol2bdnCh)
		}
	}

	if g.noValidator {
		go g.sendAliveMessages()
	} else {
		go g.nl.Recv(sol2bdnCh)
	}
}

func (g *Gateway) sendAliveMessages() {
	ticker := time.NewTicker(aliveMsgInterval)
	defer ticker.Stop()

	for {
		if _, err := g.conn.WriteToUDP([]byte(solana.AliveMsg), g.bdnUDPAddr); err != nil {
			g.lg.Errorf("sendAliveMessages: write to UDP: %v", err)
		}

		<-ticker.C
	}
}

func (g *Gateway) receiveShredsFromBDN(broadcastCh chan []byte) {
	var done = g.ctx.Done()

	for i := 0; ; i++ {
		select {
		case <-done:
			return
		default:
		}

		var buf = g.pool.Get().([]byte)
		_, addr, err := g.conn.ReadFromUDP(buf)
		if err != nil {
			g.lg.Errorf("read from udp: %s", err)
			g.pool.Put(buf)
			continue
		}

		if !addr.IP.Equal(g.bdnUDPAddr.IP) {
			g.pool.Put(buf)
			continue
		}

		g.bdnRegisterInactivityTrigger.Notify()

		shred, err := solana.ParseShredPartial(buf)
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

		g.stats.RecordNewShred(addr)

		if !g.cache.Set(solana.ShredKey(shred.Slot, shred.Index, shred.Variant)) {
			g.pool.Put(buf)
			continue
		}

		g.stats.RecordUnseenShred(addr, shred)

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

func (g *Gateway) broadcastToSolana(ch <-chan []byte) {
	if g.passiveMode {
		return
	}

	for buf := range ch {
		if !g.noValidator {
			if _, err := g.conn.WriteToUDP(buf, g.solana); err != nil {
				g.lg.Errorf("broadcastToSolana: write to UDP: %s", err)
			}
		}
		for _, addr := range g.addressesToSendShreds {
			_, _ = g.conn.WriteToUDP(buf, addr)
		}

		g.pool.Put(buf)
	}
}

func (g *Gateway) broadcastToBDN(ch <-chan *solana.PartialShred) {
	for shred := range ch {
		g.bdnUDPAddrMx.RLock()
		if g.bdnUDPAddr == nil { // wait until registration is done, otherwise the udp address is nil which leads to panic
			g.bdnUDPAddrMx.RUnlock()
			continue
		}
		_, err := g.conn.WriteToUDP(shred.Raw, g.bdnUDPAddr)
		g.bdnUDPAddrMx.RUnlock()

		if !g.broadcastFromBdnOnly {
			for _, addr := range g.addressesToSendShreds {
				_, _ = g.conn.WriteToUDP(shred.Raw, addr)
			}
		}

		if err != nil {
			g.lg.Errorf("broadcastToBDN: write to UDP addr: %s: %s", err)
		}
	}
}
