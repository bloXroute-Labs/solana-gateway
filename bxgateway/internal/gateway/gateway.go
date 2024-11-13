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
	udpShredSize = 1228
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

	passiveMode bool
}

type Option func(*Gateway)

func PassiveMode() Option { return func(g *Gateway) { g.passiveMode = true } }

func New(ctx context.Context, lg logger.Logger, cache *cache.AlterKey, conn *net.UDPConn, solana *net.UDPAddr, bdnHost string, bdnUDPPort int, stats *bdn.Stats, nl *netlisten.NetworkListener, registrar Registrar, opts ...Option) (*Gateway, error) {
	bdnUDPAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", bdnHost, bdnUDPPort))
	if err != nil {
		return nil, fmt.Errorf("resolve BDN UDP address: %s", err)
	}

	gw := &Gateway{
		ctx:    ctx,
		lg:     lg,
		cache:  cache,
		stats:  stats,
		nl:     nl,
		pool:   &sync.Pool{New: func() interface{} { return make([]byte, udpShredSize) }},
		conn:   conn,
		solana: solana,

		bdnUDPAddrMx: &sync.RWMutex{},
		bdnUDPAddr:   bdnUDPAddr,
		passiveMode:  false,
		registrar:    registrar,
	}

	gw.bdnRegisterInactivityTrigger = inactivitytrigger.NewInactivityTrigger(ctx, func() {
		if err := registrar.Register(); err != nil {
			lg.Errorf("retry register due to bdn inactivity failed: %s", err)
		}

		bdnUDPAddr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", bdnHost, bdnUDPPort))
		if err != nil {
			lg.Errorf("resolve BDN UDP address: %s", err)
		}

		gw.bdnUDPAddrMx.Lock()
		gw.bdnUDPAddr = bdnUDPAddr
		gw.bdnUDPAddrMx.Unlock()
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

	if err := g.registrar.Register(); err != nil {
		// This is fine until we update relays
		g.lg.Warnf("failed register to bdn during start: %s", err)
	}

	g.bdnRegisterInactivityTrigger.Start()

	for i := 0; i < runtime.NumCPU(); i++ {
		go g.receiveShredsFromBDN(bdn2solCh)
	}

	for i := 0; i < runtime.NumCPU()/2; i++ {
		go g.broadcastToSolana(bdn2solCh)
		go g.broadcastToBDN(sol2bdnCh)
	}

	go g.nl.Recv(sol2bdnCh)
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
		if _, err := g.conn.WriteToUDP(buf, g.solana); err != nil {
			g.lg.Errorf("broadcastToSolana: write to UDP: %s", err)
		}

		g.pool.Put(buf)
	}
}

func (g *Gateway) broadcastToBDN(ch <-chan *solana.PartialShred) {
	for shred := range ch {
		g.bdnUDPAddrMx.RLock()
		_, err := g.conn.WriteToUDP(shred.Raw, g.bdnUDPAddr)
		g.bdnUDPAddrMx.RUnlock()

		if err != nil {
			g.lg.Errorf("broadcastToBDN: write to UDP addr: %s: %s", err)
		}
	}
}
