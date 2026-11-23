package cache

import (
	"sync"
	"time"

	"linkpulse/internal/store"
)

// LinkCache is deliberately small. A Redis implementation can be added later
// without changing the HTTP handlers.
type LinkCache interface {
	Get(code string) (store.Link, bool)
	Set(link store.Link)
	Delete(code string)
}

type item struct {
	link      store.Link
	expiresAt time.Time
}

// Memory is an in-process TTL cache. It is free and needs no setup for the
// local MVP. In production this is the place where Redis/ElastiCache fits.
type Memory struct {
	mu    sync.RWMutex
	ttl   time.Duration
	items map[string]item
}

func NewMemory(ttl time.Duration) *Memory {
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Memory{
		ttl:   ttl,
		items: make(map[string]item),
	}
}

func (c *Memory) Get(code string) (store.Link, bool) {
	c.mu.RLock()
	entry, ok := c.items[code]
	c.mu.RUnlock()
	if !ok {
		return store.Link{}, false
	}
	if time.Now().After(entry.expiresAt) {
		c.Delete(code)
		return store.Link{}, false
	}
	return entry.link, true
}

func (c *Memory) Set(link store.Link) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[link.Code] = item{link: link, expiresAt: time.Now().Add(c.ttl)}
}

func (c *Memory) Delete(code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, code)
}
