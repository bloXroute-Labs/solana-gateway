package cache

import (
	"sync"
	"time"
)

// AlterKey cache is aimed to record unique keys only with no values
// Cleanup is done it 2phases so after first clean previous values are
// preserved and may be accessed until the second one.
type AlterKey struct {
	current map[uint64]struct{}
	altered map[uint64]struct{}
	mx      sync.Mutex
}

func NewAlterKey(cleanTimeout time.Duration) *AlterKey {
	// Pre-allocate for expected load: 3.5k shreds/sec * 60sec = ~210k entries
	expectedSize := 250000
	c := &AlterKey{
		current: make(map[uint64]struct{}, expectedSize),
		altered: make(map[uint64]struct{}, expectedSize),
		mx:      sync.Mutex{},
	}

	go c.cleanup(cleanTimeout)
	return c
}

// Set sets key into cache and returns true if value was set.
//
// Before setting the key it checks if it is present in either
// current or altered cache and if so - returns false
func (c *AlterKey) Set(key uint64) bool {
	c.mx.Lock()
	defer c.mx.Unlock()

	// first check current, altered is old entries that are not yet cleaned up
	if _, ok := c.current[key]; ok {
		return false
	}

	if _, ok := c.altered[key]; ok {
		return false
	}

	c.current[key] = struct{}{}
	return true
}

func (c *AlterKey) cleanup(timeout time.Duration) bool {
	for {
		time.Sleep(timeout)
		c.mx.Lock()
		clear(c.altered) // Reuse backing array, no allocation
		c.altered, c.current = c.current, c.altered
		c.mx.Unlock()
	}
}
