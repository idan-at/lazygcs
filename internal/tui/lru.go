package tui

import (
	"container/list"
	"sync"
)

// LRUCache is a generic thread-safe LRU cache.
type LRUCache[K comparable, V any] struct {
	maxEntries int
	ll         *list.List
	cache      map[K]*list.Element
	mu         sync.RWMutex
}

type entry[K comparable, V any] struct {
	key   K
	value V
}

// NewLRUCache creates a new LRUCache.
func NewLRUCache[K comparable, V any](maxEntries int) *LRUCache[K, V] {
	return &LRUCache[K, V]{
		maxEntries: maxEntries,
		ll:         list.New(),
		cache:      make(map[K]*list.Element),
	}
}

// Add adds a value to the cache.
func (c *LRUCache[K, V]) Add(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ee, ok := c.cache[key]; ok {
		c.ll.MoveToFront(ee)
		ee.Value.(*entry[K, V]).value = value
		return
	}

	ele := c.ll.PushFront(&entry[K, V]{key, value})
	c.cache[key] = ele

	if c.maxEntries != 0 && c.ll.Len() > c.maxEntries {
		c.removeOldest()
	}
}

// Get looks up a key's value from the cache.
func (c *LRUCache[K, V]) Get(key K) (value V, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ele, hit := c.cache[key]; hit {
		c.ll.MoveToFront(ele)
		return ele.Value.(*entry[K, V]).value, true
	}
	return
}

// Peek looks up a key's value from the cache without updating the LRU order.
func (c *LRUCache[K, V]) Peek(key K) (value V, ok bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if ele, hit := c.cache[key]; hit {
		return ele.Value.(*entry[K, V]).value, true
	}
	return
}

// Remove removes the provided key from the cache.
func (c *LRUCache[K, V]) Remove(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ele, hit := c.cache[key]; hit {
		c.removeElement(ele)
	}
}

// Purge clears all items from the cache.
func (c *LRUCache[K, V]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ll = list.New()
	c.cache = make(map[K]*list.Element)
}

func (c *LRUCache[K, V]) removeOldest() {
	ele := c.ll.Back()
	if ele != nil {
		c.removeElement(ele)
	}
}

func (c *LRUCache[K, V]) removeElement(e *list.Element) {
	c.ll.Remove(e)
	kv := e.Value.(*entry[K, V])
	delete(c.cache, kv.key)
}

// Len returns the number of items in the cache.
func (c *LRUCache[K, V]) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ll.Len()
}
