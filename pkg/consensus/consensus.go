package consensus

import (
	"context"
	"sync"
	"time"
)

// entry stores the value along with its timestamp.
type entry[V comparable] struct {
	value   V
	updated bool
}

// Consensus is a simple consensus algorithm that calculates the majority value.
type Consensus[K comparable, V comparable] struct {
	ctx          context.Context
	data         map[K]*entry[V]
	value        V // Current consensus value
	tickInterval time.Duration
	minValues    int
	dropOldKeys  bool
	mu           sync.RWMutex
}

// New creates a new Consensus instance.
func New[K comparable, V comparable](ctx context.Context, tickInterval time.Duration, minValues int, dropOldKeys bool) *Consensus[K, V] {
	c := &Consensus[K, V]{
		ctx:          ctx,
		data:         make(map[K]*entry[V]),
		tickInterval: tickInterval,
		minValues:    minValues,
		dropOldKeys:  dropOldKeys,
	}
	go c.run()
	return c
}

// Set updates the map with a new value and timestamp.
func (c *Consensus[K, V]) Set(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if v, ok := c.data[key]; ok {
		v.value = value
		v.updated = true
	} else {
		c.data[key] = &entry[V]{value: value, updated: true}
	}
}

// Get returns the current consensus value.
func (c *Consensus[K, V]) Get() V {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.value
}

// run periodically reconciles the consensus value.
func (c *Consensus[K, V]) run() {
	ticker := time.NewTicker(c.tickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.reconcile()
		case <-c.ctx.Done():
			return
		}
	}
}

// reconcile calculates the majority value and updates the consensus value.
func (c *Consensus[K, V]) reconcile() {
	c.mu.Lock()
	defer c.mu.Unlock()

	var (
		frequency     = make(map[V]int)
		maxCount      int
		majorityValue V
		validCount    int
	)

	// Remove expired values and count occurrences of valid ones
	for k, e := range c.data {
		if !e.updated {
			if c.dropOldKeys {
				delete(c.data, k) // Remove expired
			}
			continue
		}

		e.updated = false

		frequency[e.value]++
		validCount++
		if frequency[e.value] > maxCount {
			maxCount = frequency[e.value]
			majorityValue = e.value
		}
	}

	// Only update consensus if minimum participants threshold is met
	if validCount >= c.minValues {
		c.value = majorityValue
	} else {
		var zero V
		c.value = zero
	}
}
