package tui

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestLRUCache(t *testing.T) {
	t.Run("basic operations", func(t *testing.T) {
		cache := NewLRUCache[string, int](2)

		cache.Add("one", 1)
		cache.Add("two", 2)

		val, ok := cache.Get("one")
		assert.Assert(t, ok)
		assert.Equal(t, val, 1)

		assert.Equal(t, cache.Len(), 2)
	})

	t.Run("peek", func(t *testing.T) {
		cache := NewLRUCache[string, int](2)

		cache.Add("one", 1)
		cache.Add("two", 2)

		// Peek should not update LRU order
		val, ok := cache.Peek("one")
		assert.Assert(t, ok)
		assert.Equal(t, val, 1)

		// Adding a third should evict the least recently used ("one", because we only peeked at it)
		cache.Add("three", 3)

		_, ok = cache.Get("one")
		assert.Assert(t, !ok, "key 'one' should have been evicted")

		val, ok = cache.Get("two")
		assert.Assert(t, ok)
		assert.Equal(t, val, 2)
	})

	t.Run("eviction", func(t *testing.T) {
		cache := NewLRUCache[string, int](2)

		cache.Add("one", 1)
		cache.Add("two", 2)
		cache.Add("three", 3) // Should evict "one"

		_, ok := cache.Get("one")
		assert.Assert(t, !ok, "key 'one' should have been evicted")

		val, ok := cache.Get("two")
		assert.Assert(t, ok)
		assert.Equal(t, val, 2)

		val, ok = cache.Get("three")
		assert.Assert(t, ok)
		assert.Equal(t, val, 3)
	})

	t.Run("update existing", func(t *testing.T) {
		cache := NewLRUCache[string, int](2)

		cache.Add("one", 1)
		cache.Add("two", 2)
		cache.Add("one", 10) // Update "one" and move to front

		cache.Add("three", 3) // Should evict "two"

		_, ok := cache.Get("two")
		assert.Assert(t, !ok, "key 'two' should have been evicted")

		val, ok := cache.Get("one")
		assert.Assert(t, ok)
		assert.Equal(t, val, 10)
	})

	t.Run("remove", func(t *testing.T) {
		cache := NewLRUCache[string, int](2)
		cache.Add("one", 1)
		cache.Remove("one")

		_, ok := cache.Get("one")
		assert.Assert(t, !ok)
	})

	t.Run("purge", func(t *testing.T) {
		cache := NewLRUCache[string, int](2)
		cache.Add("one", 1)
		cache.Add("two", 2)
		cache.Purge()

		assert.Equal(t, cache.Len(), 0)
	})
}
