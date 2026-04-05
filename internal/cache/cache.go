package cache

import (
	"sync"
	"time"
)

// Entry holds a cached value with metadata.
type Entry struct {
	Data      []byte
	FetchedAt time.Time
	Dirty     bool
}

// Cache is a thread-safe in-memory cache for file contents.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*Entry
	ttl     time.Duration
	// ZenMode disables TTL expiry — once fetched, entries stay cached
	// until the process exits. Only dirty tracking still applies.
	ZenMode bool
}

// New creates a new cache with the given TTL.
func New(ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]*Entry),
		ttl:     ttl,
	}
}

// Get returns the cached data for a key, or nil if not found/expired.
// In ZenMode, cached entries never expire.
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	// In zen mode, once fetched, always serve from cache
	if c.ZenMode {
		return e.Data, true
	}
	if !e.Dirty && time.Since(e.FetchedAt) > c.ttl {
		return nil, false
	}
	return e.Data, true
}

// Set stores data in the cache.
func (c *Cache) Set(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &Entry{
		Data:      data,
		FetchedAt: time.Now(),
	}
}

// SetDirty stores data and marks it as dirty (locally modified, not yet synced).
func (c *Cache) SetDirty(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = &Entry{
		Data:      data,
		FetchedAt: time.Now(),
		Dirty:     true,
	}
}

// ClearDirty marks a cache entry as clean (synced to remote).
func (c *Cache) ClearDirty(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.entries[key]; ok {
		e.Dirty = false
		e.FetchedAt = time.Now()
	}
}

// IsDirty returns whether a cache entry has unsaved local modifications.
func (c *Cache) IsDirty(key string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if e, ok := c.entries[key]; ok {
		return e.Dirty
	}
	return false
}

// Delete removes an entry from the cache.
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.entries, key)
}

// DirtyKeys returns all keys that have unsaved modifications.
func (c *Cache) DirtyKeys() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var keys []string
	for k, e := range c.entries {
		if e.Dirty {
			keys = append(keys, k)
		}
	}
	return keys
}
