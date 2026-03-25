/*
   Copyright (C) 2026 FSKY <development@fsky.io>

   This file is part of Mezzo

   Mezzo is free software: you can redistribute it and/or modify it under the
   terms of the GNU Affero General Public License as published by the Free
   Software Foundation, either version 3 of the License, or (at your option) any
   later version.

   This program is distributed in the hope that it will be useful, but WITHOUT
   ANY WARRANTY; without even the implied warranty of  MERCHANTABILITY or
   FITNESS FOR A PARTICULAR PURPOSE.  See the GNU Affero General Public License
   for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package cache

import (
	"sync"
	"time"
)

// entry holds a cached value along with its expiration time.
type entry[V any] struct {
	value     V
	expiresAt time.Time
}

// Cache is a generic in-memory TTL cache backed by a map and a read-write
// mutex. It is safe for concurrent use.
type Cache[V any] struct {
	mu      sync.RWMutex
	items   map[string]entry[V]
	ttl     time.Duration
	maxSize int
	stop    chan struct{}
}

// New creates a new Cache with the given TTL and maximum number of entries.
// A background goroutine periodically evicts expired entries.
func New[V any](ttl time.Duration, maxSize int) *Cache[V] {
	c := &Cache[V]{
		items:   make(map[string]entry[V]),
		ttl:     ttl,
		maxSize: maxSize,
		stop:    make(chan struct{}),
	}

	go c.reap()
	return c
}

// Get retrieves a value from the cache. The second return value indicates
// whether the key was found and has not expired.
func (c *Cache[V]) Get(key string) (V, bool) {
	c.mu.RLock()
	e, ok := c.items[key]
	c.mu.RUnlock()

	if !ok || time.Now().After(e.expiresAt) {
		var zero V
		return zero, false
	}

	return e.value, true
}

// Set stores a value in the cache. If the cache is at capacity, the oldest
// entry (by expiration time) is evicted to make room.
func (c *Cache[V]) Set(key string, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict the entry with the earliest expiration if at capacity.
	// We only need to evict when adding a genuinely new key.
	if _, exists := c.items[key]; !exists && len(c.items) >= c.maxSize {
		c.evictOldest()
	}

	c.items[key] = entry[V]{
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Len returns the number of entries currently in the cache (including expired
// entries that have not yet been reaped).
func (c *Cache[V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Stop terminates the background reaper goroutine.
func (c *Cache[V]) Stop() {
	close(c.stop)
}

// evictOldest removes the entry with the earliest expiration time.
// Must be called with c.mu held.
func (c *Cache[V]) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for k, e := range c.items {
		if oldestKey == "" || e.expiresAt.Before(oldestTime) {
			oldestKey = k
			oldestTime = e.expiresAt
		}
	}

	if oldestKey != "" {
		delete(c.items, oldestKey)
	}
}

// reap periodically removes expired entries from the cache.
func (c *Cache[V]) reap() {
	ticker := time.NewTicker(c.ttl / 2)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			now := time.Now()
			for k, e := range c.items {
				if now.After(e.expiresAt) {
					delete(c.items, k)
				}
			}
			c.mu.Unlock()
		case <-c.stop:
			return
		}
	}
}
