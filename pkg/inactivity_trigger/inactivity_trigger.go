package inactivitytrigger

import (
	"context"
	"time"
)

type InactivityTrigger struct {
	ctx      context.Context
	callback func()
	ch       chan struct{}
	dur      time.Duration
}

func NewInactivityTrigger(ctx context.Context, callback func(), dur time.Duration) *InactivityTrigger {
	trigger := &InactivityTrigger{
		ctx:      ctx,
		callback: callback,
		ch:       make(chan struct{}),
		dur:      dur,
	}

	return trigger
}

func (t *InactivityTrigger) Start() {
	go t.routine()
}

func (t *InactivityTrigger) Notify() {
	select {
	case t.ch <- struct{}{}:
	default:
	}
}

func (t *InactivityTrigger) routine() {
	ticker := time.NewTicker(t.dur)

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-t.ch:
			ticker.Reset(t.dur)
		case <-ticker.C:
			t.callback()
		}
	}
}
