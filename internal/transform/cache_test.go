package transform

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTemplateCache(t *testing.T) {
	// Test with default values
	cache := NewTemplateCache(0, 0)
	assert.NotNil(t, cache)
	assert.Equal(t, 100, cache.maxSize)
	assert.Equal(t, time.Hour, cache.ttl)

	// Test with custom values
	cache2 := NewTemplateCache(50, 30*time.Minute)
	assert.NotNil(t, cache2)
	assert.Equal(t, 50, cache2.maxSize)
	assert.Equal(t, 30*time.Minute, cache2.ttl)
}

func TestTemplateCache_Get(t *testing.T) {
	cache := NewTemplateCache(10, time.Hour)

	// Test cache miss - create new engine
	engine1, err := cache.Get(EngineTypeGoTemplate, `{{ .Status }}`)
	require.NoError(t, err)
	assert.NotNil(t, engine1)
	assert.Equal(t, 1, cache.Size())

	// Test cache hit - get existing engine
	engine2, err := cache.Get(EngineTypeGoTemplate, `{{ .Status }}`)
	require.NoError(t, err)
	assert.NotNil(t, engine2)
	assert.Equal(t, 1, cache.Size())

	// Different template should create new entry
	engine3, err := cache.Get(EngineTypeGoTemplate, `{{ .Version }}`)
	require.NoError(t, err)
	assert.NotNil(t, engine3)
	assert.Equal(t, 2, cache.Size())

	// Test invalid template
	_, err = cache.Get(EngineTypeGoTemplate, `{{ .Status }`)
	assert.Error(t, err)
}

func TestTemplateCache_Clear(t *testing.T) {
	cache := NewTemplateCache(10, time.Hour)

	// Add some entries
	_, _ = cache.Get(EngineTypeGoTemplate, `{{ .Status }}`)
	_, _ = cache.Get(EngineTypeGoTemplate, `{{ .Version }}`)
	assert.Equal(t, 2, cache.Size())

	// Clear cache
	cache.Clear()
	assert.Equal(t, 0, cache.Size())
}

func TestTemplateCache_Eviction(t *testing.T) {
	cache := NewTemplateCache(3, time.Hour)

	// Fill cache to capacity
	_, _ = cache.Get(EngineTypeGoTemplate, `{{ .Status }}`)
	time.Sleep(10 * time.Millisecond)
	_, _ = cache.Get(EngineTypeGoTemplate, `{{ .Version }}`)
	time.Sleep(10 * time.Millisecond)
	_, _ = cache.Get(EngineTypeGoTemplate, `{{ .Receiver }}`)
	assert.Equal(t, 3, cache.Size())

	// Adding one more should evict the oldest
	_, _ = cache.Get(EngineTypeGoTemplate, `{{ .GroupKey }}`)
	assert.Equal(t, 3, cache.Size())
}

func TestTemplateCache_Stats(t *testing.T) {
	cache := NewTemplateCache(10, time.Hour)

	// Add some entries
	_, _ = cache.Get(EngineTypeGoTemplate, `{{ .Status }}`)
	_, _ = cache.Get(EngineTypeGoTemplate, `{{ .Status }}`) // Hit
	_, _ = cache.Get(EngineTypeGoTemplate, `{{ .Version }}`)

	stats := cache.Stats()
	assert.Equal(t, 2, stats.Size)
	assert.Equal(t, 10, stats.MaxSize)
	assert.Equal(t, time.Hour, stats.TTL)
	assert.Equal(t, uint64(3), stats.TotalHits)
	assert.Len(t, stats.Entries, 2)
}

func TestTemplateCache_Expiration(t *testing.T) {
	cache := NewTemplateCache(10, 100*time.Millisecond)

	// Add entry
	_, err := cache.Get(EngineTypeGoTemplate, `{{ .Status }}`)
	require.NoError(t, err)
	assert.Equal(t, 1, cache.Size())

	// Wait for expiration
	time.Sleep(150 * time.Millisecond)

	// Access should create new entry
	_, err = cache.Get(EngineTypeGoTemplate, `{{ .Status }}`)
	require.NoError(t, err)

	// Manually trigger cleanup
	cache.cleanup()

	// Size should still be 1 (old expired entry removed, new one added)
	assert.Equal(t, 1, cache.Size())
}

func TestTemplateCache_GenerateKey(t *testing.T) {
	cache := NewTemplateCache(10, time.Hour)

	// Same input should generate same key
	key1 := cache.generateKey(EngineTypeGoTemplate, `{{ .Status }}`)
	key2 := cache.generateKey(EngineTypeGoTemplate, `{{ .Status }}`)
	assert.Equal(t, key1, key2)

	// Different template should generate different key
	key3 := cache.generateKey(EngineTypeGoTemplate, `{{ .Version }}`)
	assert.NotEqual(t, key1, key3)

	// Different engine type should generate different key
	key4 := cache.generateKey(EngineTypeJQ, `{{ .Status }}`)
	assert.NotEqual(t, key1, key4)
}
