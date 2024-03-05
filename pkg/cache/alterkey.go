package cache

import (
	"sync"
	"time"
)

// AlterKey cache is aimed to record unique keys only with no values
// Cleanup is done it 2phases so after first clean previous values are
// preserved and may be accessed until the second one.
type AlterKey struct {
	current map[string]struct{}
	altered map[string]struct{}
	mx      sync.Mutex
}

func NewAlterKey(cleanTimeout time.Duration) *AlterKey {
	c := &AlterKey{
		current: make(map[string]struct{}),
		altered: make(map[string]struct{}),
		mx:      sync.Mutex{},
	}

	go c.cleanup(cleanTimeout)
	return c
}

// Set sets key into cache and returns true if value was set.
//
// Before setting the key it checks if it is present in either
// current or altered cache and if so - returns false
func (c *AlterKey) Set(key string) bool {
	c.mx.Lock()
	defer c.mx.Unlock()

	if _, ok := c.altered[key]; ok {
		return false
	}

	if _, ok := c.current[key]; ok {
		return false
	}

	c.current[key] = struct{}{}
	return true
}

func (c *AlterKey) cleanup(timeout time.Duration) bool {
	for {
		time.Sleep(timeout)
		c.mx.Lock()
		c.altered = c.current
		c.current = make(map[string]struct{}, len(c.current))
		c.mx.Unlock()
	}
}
