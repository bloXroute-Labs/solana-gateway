package slot_monitor

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
	"github.com/gagliardetto/solana-go/rpc/ws"
)

const (
	healthy health = iota
	disconnected
)

type health int32

func (h health) String() string {
	return [...]string{
		healthy:      "HEALTHY",
		disconnected: "DISCONNECTED",
	}[h]
}

// SlotMonitor knows the current slot based on validator subscription
type SlotMonitor struct {
	ctx         context.Context
	l           logger.Logger
	pubsub      string
	health      atomic.Int32
	currentSlot atomic.Uint64
}

func NewSlotMonitor(ctx context.Context, l logger.Logger, pubsub string) *SlotMonitor {
	sm := &SlotMonitor{
		ctx:         ctx,
		l:           l,
		pubsub:      pubsub,
		currentSlot: atomic.Uint64{},
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go sm.run(&wg)
	wg.Wait()

	return sm
}

func (s *SlotMonitor) CurrentSlot() (uint64, error) {
	if h := s.health.Load(); h != int32(healthy) {
		return 0, fmt.Errorf("service is unhealthy: %s", health(h).String())
	}

	return s.currentSlot.Load(), nil
}

func (s *SlotMonitor) run(wg *sync.WaitGroup) {
	for startup := true; ; time.Sleep(time.Second) {
		select {
		case <-s.ctx.Done():
			s.l.Info("slot_monitor: recv ctx cancellation signal, exiting")
			return
		default:
		}

		client, err := ws.Connect(s.ctx, s.pubsub)
		if err != nil {
			s.l.Errorf("failed to connect to rpc pubsub, retrying... %s", err)
			s.health.Store(int32(disconnected))
			continue
		}

		s.l.Info("established ws client connection")

		sub, err := client.SlotsUpdatesSubscribe()
		if err != nil {
			s.l.Errorf("SlotsUpdatesSubscribe: failed to connect to rpc pubsub, retrying... %s", err)
			s.health.Store(int32(disconnected))
			client.Close()
			continue
		}

		for {
			select {
			case <-s.ctx.Done():
				s.l.Info("slot_monitor: recv ctx cancellation signal, exiting")
				return
			default:
			}

			upd, err := sub.Recv()
			if err != nil {
				s.l.Errorf("SlotsUpdatesSubscribe: recv error, reconnecting... %s", err)
				s.health.Store(int32(disconnected))
				break
			}

			switch upd.Type {
			case ws.SlotsUpdatesCompleted:
				s.health.Store(int32(healthy))
				// This update indicates that a full slot was received by the connected
				// node, so we can stop sending transactions to the leader for that slot
				prev := s.currentSlot.Load()
				next := upd.Slot + 1
				if prev > next {
					continue
				}

				s.currentSlot.Store(next)
				s.l.Tracef("slot_monitor: store slot: %d", next)
				if startup {
					wg.Done()
					startup = false
				}
			default:
				continue
			}
		}
	}
}
