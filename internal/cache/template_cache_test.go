package cache

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewTemplateCache(t *testing.T) {
	tests := []struct {
		name        string
		maxSize     int
		ttl         time.Duration
		expectedMax int
		expectedTTL time.Duration
	}{
		{
			name:        "valid parameters",
			maxSize:     50,
			ttl:         30 * time.Minute,
			expectedMax: 50,
			expectedTTL: 30 * time.Minute,
		},
		{
			name:        "zero maxSize uses default",
			maxSize:     0,
			ttl:         30 * time.Minute,
			expectedMax: 100,
			expectedTTL: 30 * time.Minute,
		},
		{
			name:        "negative maxSize uses default",
			maxSize:     -10,
			ttl:         30 * time.Minute,
			expectedMax: 100,
			expectedTTL: 30 * time.Minute,
		},
		{
			name:        "zero TTL uses default",
			maxSize:     50,
			ttl:         0,
			expectedMax: 50,
			expectedTTL: 1 * time.Hour,
		},
		{
			name:        "negative TTL uses default",
			maxSize:     50,
			ttl:         -10 * time.Minute,
			expectedMax: 50,
			expectedTTL: 1 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewTemplateCache(tt.maxSize, tt.ttl)
			assert.Equal(t, tt.expectedMax, cache.maxSize)
			assert.Equal(t, tt.expectedTTL, cache.ttl)
			assert.NotNil(t, cache.cache)
			assert.NotNil(t, cache.lru)
		})
	}
}

func TestTemplateCache_SetAndGet(t *testing.T) {
	cache := NewTemplateCache(5, 1*time.Hour)

	// Test setting and getting values
	cache.Set("key1", "value1")
	cache.Set("key2", 42)
	cache.Set("key3", []string{"a", "b", "c"})

	// Test getting existing values
	val1, ok1 := cache.Get("key1")
	assert.True(t, ok1)
	assert.Equal(t, "value1", val1)

	val2, ok2 := cache.Get("key2")
	assert.True(t, ok2)
	assert.Equal(t, 42, val2)

	val3, ok3 := cache.Get("key3")
	assert.True(t, ok3)
	assert.Equal(t, []string{"a", "b", "c"}, val3)

	// Test getting non-existent value
	val4, ok4 := cache.Get("nonexistent")
	assert.False(t, ok4)
	assert.Nil(t, val4)

	// Check stats
	stats := cache.Stats()
	assert.Equal(t, int64(3), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, 3, stats.TotalSize)
}

func TestTemplateCache_Update(t *testing.T) {
	cache := NewTemplateCache(5, 1*time.Hour)

	// Set initial value
	cache.Set("key1", "value1")

	// Update the value
	cache.Set("key1", "updated_value")

	val, ok := cache.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "updated_value", val)

	// Should still have only one entry
	stats := cache.Stats()
	assert.Equal(t, 1, stats.TotalSize)
}

func TestTemplateCache_LRUEviction(t *testing.T) {
	cache := NewTemplateCache(3, 1*time.Hour)

	// Fill cache to capacity
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	// Add one more to trigger eviction
	cache.Set("key4", "value4")

	// key1 should be evicted (oldest)
	_, ok1 := cache.Get("key1")
	assert.False(t, ok1)

	// Others should still exist
	_, ok2 := cache.Get("key2")
	assert.True(t, ok2)

	_, ok3 := cache.Get("key3")
	assert.True(t, ok3)

	_, ok4 := cache.Get("key4")
	assert.True(t, ok4)

	stats := cache.Stats()
	assert.Equal(t, int64(1), stats.Evictions)
	assert.Equal(t, 3, stats.TotalSize)
}

func TestTemplateCache_LRUAccess(t *testing.T) {
	cache := NewTemplateCache(3, 1*time.Hour)

	// Fill cache
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	// Access key1 to make it most recently used
	cache.Get("key1")

	// Add new key, key2 should be evicted (least recently used)
	cache.Set("key4", "value4")

	// key2 should be evicted
	_, ok2 := cache.Get("key2")
	assert.False(t, ok2)

	// key1 should still exist (was accessed recently)
	_, ok1 := cache.Get("key1")
	assert.True(t, ok1)
}

func TestTemplateCache_Delete(t *testing.T) {
	cache := NewTemplateCache(5, 1*time.Hour)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	// Delete existing key
	deleted := cache.Delete("key1")
	assert.True(t, deleted)

	// Key should no longer exist
	_, ok := cache.Get("key1")
	assert.False(t, ok)

	// Other key should still exist
	_, ok2 := cache.Get("key2")
	assert.True(t, ok2)

	// Delete non-existent key
	deleted2 := cache.Delete("nonexistent")
	assert.False(t, deleted2)

	stats := cache.Stats()
	assert.Equal(t, int64(1), stats.Evictions)
	assert.Equal(t, 1, stats.TotalSize)
}

func TestTemplateCache_Clear(t *testing.T) {
	cache := NewTemplateCache(5, 1*time.Hour)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	cache.Clear()

	// All keys should be gone
	_, ok1 := cache.Get("key1")
	assert.False(t, ok1)

	_, ok2 := cache.Get("key2")
	assert.False(t, ok2)

	_, ok3 := cache.Get("key3")
	assert.False(t, ok3)

	stats := cache.Stats()
	assert.Equal(t, 0, stats.TotalSize)
}

func TestTemplateCache_TTLExpiration(t *testing.T) {
	cache := NewTemplateCache(5, 100*time.Millisecond)

	cache.Set("key1", "value1")

	// Should be available immediately
	_, ok := cache.Get("key1")
	assert.True(t, ok)

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Should be expired
	_, ok = cache.Get("key1")
	assert.False(t, ok)

	stats := cache.Stats()
	assert.Equal(t, int64(1), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
}

func TestTemplateCache_Purge(t *testing.T) {
	cache := NewTemplateCache(5, 100*time.Millisecond)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Add new key (should not be expired)
	cache.Set("key3", "value3")

	// Purge expired entries
	removed := cache.Purge()
	assert.Equal(t, 2, removed)

	// key3 should still exist
	_, ok := cache.Get("key3")
	assert.True(t, ok)

	// key1 and key2 should be gone
	_, ok1 := cache.Get("key1")
	assert.False(t, ok1)

	_, ok2 := cache.Get("key2")
	assert.False(t, ok2)

	stats := cache.Stats()
	assert.Equal(t, 1, stats.TotalSize)
}

func TestTemplateCache_PurgeWithZeroTTL(t *testing.T) {
	cache := NewTemplateCache(5, 0) // No TTL

	cache.Set("key1", "value1")

	removed := cache.Purge()
	assert.Equal(t, 0, removed)

	// Key should still exist
	_, ok := cache.Get("key1")
	assert.True(t, ok)
}

func TestTemplateCache_EvictFunc(t *testing.T) {
	cache := NewTemplateCache(2, 1*time.Hour)

	var evictedKeys []string
	var evictedValues []interface{}

	cache.SetEvictFunc(func(key string, value interface{}) {
		evictedKeys = append(evictedKeys, key)
		evictedValues = append(evictedValues, value)
	})

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3") // Should evict key1

	assert.Equal(t, []string{"key1"}, evictedKeys)
	assert.Equal(t, []interface{}{"value1"}, evictedValues)

	// Test evict func on delete
	cache.Delete("key2")

	assert.Equal(t, []string{"key1", "key2"}, evictedKeys)
	assert.Equal(t, []interface{}{"value1", "value2"}, evictedValues)
}

func TestTemplateCache_EvictFuncOnClear(t *testing.T) {
	cache := NewTemplateCache(5, 1*time.Hour)

	var evictedKeys []string
	cache.SetEvictFunc(func(key string, _ interface{}) {
		evictedKeys = append(evictedKeys, key)
	})

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	cache.Clear()

	// All keys should have been passed to evict function
	assert.Len(t, evictedKeys, 2)
	assert.Contains(t, evictedKeys, "key1")
	assert.Contains(t, evictedKeys, "key2")
}

func TestTemplateCache_StartCleanupTask(t *testing.T) {
	cache := NewTemplateCache(5, 50*time.Millisecond)

	cache.Set("key1", "value1")

	// Start cleanup task with short interval
	done := cache.StartCleanupTask(100 * time.Millisecond)

	// Wait for cleanup to run
	time.Sleep(200 * time.Millisecond)

	// Key should be expired and cleaned up
	_, ok := cache.Get("key1")
	assert.False(t, ok)

	// Stop cleanup task
	close(done)
}

func TestTemplateCache_StartCleanupTaskWithZeroInterval(t *testing.T) {
	cache := NewTemplateCache(5, 1*time.Hour)

	// Should use default interval (5 minutes)
	done := cache.StartCleanupTask(0)
	assert.NotNil(t, done)

	// Stop immediately
	close(done)
}

func TestTemplateCache_AccessCount(t *testing.T) {
	cache := NewTemplateCache(5, 1*time.Hour)

	cache.Set("key1", "value1")

	// Access multiple times
	cache.Get("key1")
	cache.Get("key1")
	cache.Get("key1")

	// Check that access count is tracked (indirectly through stats)
	stats := cache.Stats()
	assert.Equal(t, int64(3), stats.Hits)
}

func TestTemplateCache_ConcurrentAccess(t *testing.T) {
	cache := NewTemplateCache(100, 1*time.Hour)

	var wg sync.WaitGroup
	numGoroutines := 10
	numOperations := 100

	// Concurrent writers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				value := fmt.Sprintf("value_%d_%d", id, j)
				cache.Set(key, value)
			}
		}(i)
	}

	// Concurrent readers
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key_%d_%d", id, j)
				cache.Get(key)
			}
		}(i)
	}

	wg.Wait()

	// Cache should be functional after concurrent access
	cache.Set("test", "value")
	val, ok := cache.Get("test")
	assert.True(t, ok)
	assert.Equal(t, "value", val)
}

func TestTemplateCache_Stats(t *testing.T) {
	cache := NewTemplateCache(3, 1*time.Hour)

	// Initial stats
	stats := cache.Stats()
	assert.Equal(t, int64(0), stats.Hits)
	assert.Equal(t, int64(0), stats.Misses)
	assert.Equal(t, int64(0), stats.Evictions)
	assert.Equal(t, 0, stats.TotalSize)

	// Add entries
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	// Get hits and miss
	cache.Get("key1")        // hit
	cache.Get("key2")        // hit
	cache.Get("nonexistent") // miss

	stats = cache.Stats()
	assert.Equal(t, int64(2), stats.Hits)
	assert.Equal(t, int64(1), stats.Misses)
	assert.Equal(t, int64(0), stats.Evictions)
	assert.Equal(t, 2, stats.TotalSize)

	// Trigger eviction
	cache.Set("key3", "value3")
	cache.Set("key4", "value4") // Should evict key1

	stats = cache.Stats()
	assert.Equal(t, int64(1), stats.Evictions)
	assert.Equal(t, 3, stats.TotalSize)
}
