package main

import (
	"container/list"
	"sync"
)

// CacheItem represents a cached file in memory.
type CacheItem struct {
	Key  string
	Data []byte
}

// MemoryCache implements an LRU cache limited by total memory size (bytes).
type MemoryCache struct {
	maxBytes  int64
	usedBytes int64
	ll        *list.List
	cache     map[string]*list.Element
	mu        sync.RWMutex
}

// NewMemoryCache creates a new MemoryCache with the given maximum size in bytes.
func NewMemoryCache(maxBytes int64) *MemoryCache {
	return &MemoryCache{
		maxBytes:  maxBytes,
		usedBytes: 0,
		ll:        list.New(),
		cache:     make(map[string]*list.Element),
	}
}

// Get retrieves an item from the cache.
func (c *MemoryCache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.ll.MoveToFront(elem)
		return elem.Value.(*CacheItem).Data, true
	}
	return nil, false
}

// Set adds an item to the cache and evicts older items if necessary.
// If the payload itself is larger than the max cache size, it's not cached.
func (c *MemoryCache) Set(key string, data []byte) {
	dataSize := int64(len(data))
	if dataSize > c.maxBytes {
		return // Too large to cache
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// If key already exists, update data and move to front
	if elem, ok := c.cache[key]; ok {
		c.ll.MoveToFront(elem)
		oldItem := elem.Value.(*CacheItem)
		c.usedBytes -= int64(len(oldItem.Data))
		oldItem.Data = data
		c.usedBytes += dataSize
		c.evict()
		return
	}

	// Add new item
	item := &CacheItem{Key: key, Data: data}
	elem := c.ll.PushFront(item)
	c.cache[key] = elem
	c.usedBytes += dataSize

	c.evict()
}

// evict removes the oldest items until usedBytes <= maxBytes.
// Caller must hold the write lock.
func (c *MemoryCache) evict() {
	for c.usedBytes > c.maxBytes && c.ll.Len() > 0 {
		elem := c.ll.Back()
		if elem != nil {
			c.ll.Remove(elem)
			item := elem.Value.(*CacheItem)
			delete(c.cache, item.Key)
			c.usedBytes -= int64(len(item.Data))
		}
	}
}
