package ofr

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/bloXroute-Labs/solana-gateway/pkg/logger"
)

type SuccessCounter struct {
	lg            logger.Logger
	id            string
	printInterval time.Duration
	success       atomic.Uint64
	failure       atomic.Uint64
}

func NewSuccessCounter(lg logger.Logger, id string, printInterval time.Duration) *SuccessCounter {
	return &SuccessCounter{
		lg:            lg,
		id:            id,
		printInterval: printInterval,
		success:       atomic.Uint64{},
		failure:       atomic.Uint64{},
	}
}

func (c *SuccessCounter) RecordSuccess() {
	c.success.Add(1)
}

func (c *SuccessCounter) RecordFailure() {
	c.failure.Add(1)
}

func (c *SuccessCounter) SuccessRate() float64 {
	success := c.success.Load()
	failure := c.failure.Load()

	if success+failure > 0 {
		return float64(success) / float64(success+failure)
	}

	return 0
}

func (c *SuccessCounter) Print(ctx context.Context) {
	t := time.NewTicker(c.printInterval)

	for {
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
			success := c.success.Swap(0)
			failure := c.failure.Swap(0)

			var successPercentage float64
			if success+failure > 0 {
				successPercentage = float64(success) / float64(success+failure) * 100
			}

			c.lg.Infof("%s: success: %d, failure: %d, success rate: %.2f%%", c.id, success, failure, successPercentage)
		}
	}
}
