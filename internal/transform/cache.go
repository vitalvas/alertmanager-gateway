package transform

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// TemplateCache provides a thread-safe cache for compiled templates
type TemplateCache struct {
	mu      sync.RWMutex
	engines map[string]*cacheEntry
	maxSize int
	ttl     time.Duration
}

type cacheEntry struct {
	engine    Engine
	createdAt time.Time
	lastUsed  time.Time
	hits      uint64
}

// NewTemplateCache creates a new template cache
func NewTemplateCache(maxSize int, ttl time.Duration) *TemplateCache {
	if maxSize <= 0 {
		maxSize = 100
	}
	if ttl <= 0 {
		ttl = 1 * time.Hour
	}

	cache := &TemplateCache{
		engines: make(map[string]*cacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}

	// Start cleanup goroutine
	go cache.cleanupLoop()

	return cache
}

// Get retrieves an engine from the cache or creates a new one
func (c *TemplateCache) Get(engineType EngineType, template string) (Engine, error) {
	key := c.generateKey(engineType, template)

	// Try to get from cache
	c.mu.RLock()
	entry, exists := c.engines[key]
	c.mu.RUnlock()

	if exists && !c.isExpired(entry) {
		// Update last used time and hit count
		c.mu.Lock()
		entry.lastUsed = time.Now()
		entry.hits++
		c.mu.Unlock()

		return entry.engine, nil
	}

	// Create new engine
	engine, err := NewEngine(engineType, template)
	if err != nil {
		return nil, err
	}

	// Add to cache
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check size limit and evict if necessary
	if len(c.engines) >= c.maxSize {
		c.evictOldest()
	}

	c.engines[key] = &cacheEntry{
		engine:    engine,
		createdAt: time.Now(),
		lastUsed:  time.Now(),
		hits:      1,
	}

	return engine, nil
}

// Clear removes all entries from the cache
func (c *TemplateCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.engines = make(map[string]*cacheEntry)
}

// Size returns the current number of cached engines
func (c *TemplateCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.engines)
}

// Stats returns cache statistics
func (c *TemplateCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CacheStats{
		Size:    len(c.engines),
		MaxSize: c.maxSize,
		TTL:     c.ttl,
		Entries: make([]EntryStats, 0, len(c.engines)),
	}

	for key, entry := range c.engines {
		stats.Entries = append(stats.Entries, EntryStats{
			Key:       key,
			CreatedAt: entry.createdAt,
			LastUsed:  entry.lastUsed,
			Hits:      entry.hits,
			Age:       time.Since(entry.createdAt),
		})
		stats.TotalHits += entry.hits
	}

	return stats
}

// generateKey creates a cache key from engine type and template
func (c *TemplateCache) generateKey(engineType EngineType, template string) string {
	h := sha256.New()
	h.Write([]byte(engineType))
	h.Write([]byte(":"))
	h.Write([]byte(template))
	return hex.EncodeToString(h.Sum(nil))
}

// isExpired checks if a cache entry has expired
func (c *TemplateCache) isExpired(entry *cacheEntry) bool {
	return time.Since(entry.createdAt) > c.ttl
}

// evictOldest removes the least recently used entry
func (c *TemplateCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.engines {
		if oldestKey == "" || entry.lastUsed.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.lastUsed
		}
	}

	if oldestKey != "" {
		delete(c.engines, oldestKey)
	}
}

// cleanupLoop periodically removes expired entries
func (c *TemplateCache) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes expired entries
func (c *TemplateCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for key, entry := range c.engines {
		if c.isExpired(entry) {
			delete(c.engines, key)
		}
	}
}

// CacheStats holds cache statistics
type CacheStats struct {
	Size      int
	MaxSize   int
	TTL       time.Duration
	TotalHits uint64
	Entries   []EntryStats
}

// EntryStats holds statistics for a single cache entry
type EntryStats struct {
	Key       string
	CreatedAt time.Time
	LastUsed  time.Time
	Hits      uint64
	Age       time.Duration
}
