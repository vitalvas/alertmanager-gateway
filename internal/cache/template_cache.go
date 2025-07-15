package cache

import (
	"container/list"
	"sync"
	"time"
)

// TemplateCacheEntry represents a cached template
type TemplateCacheEntry struct {
	Key         string
	Value       interface{}
	CompiledAt  time.Time
	LastUsed    time.Time
	AccessCount int64
}

// TemplateCache is an LRU cache for compiled templates
type TemplateCache struct {
	mu        sync.RWMutex
	maxSize   int
	ttl       time.Duration
	cache     map[string]*list.Element
	lru       *list.List
	stats     Stats
	evictFunc func(key string, value interface{})
}

// Stats holds cache statistics
type Stats struct {
	Hits      int64
	Misses    int64
	Evictions int64
	TotalSize int
}

// NewTemplateCache creates a new template cache
func NewTemplateCache(maxSize int, ttl time.Duration) *TemplateCache {
	if maxSize <= 0 {
		maxSize = 100 // Default to 100 entries
	}
	if ttl <= 0 {
		ttl = 1 * time.Hour // Default to 1 hour
	}

	return &TemplateCache{
		maxSize: maxSize,
		ttl:     ttl,
		cache:   make(map[string]*list.Element),
		lru:     list.New(),
	}
}

// SetEvictFunc sets the function to call when an entry is evicted
func (tc *TemplateCache) SetEvictFunc(fn func(key string, value interface{})) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.evictFunc = fn
}

// Get retrieves a value from the cache
func (tc *TemplateCache) Get(key string) (interface{}, bool) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if elem, ok := tc.cache[key]; ok {
		entry := elem.Value.(*TemplateCacheEntry)

		// Check if entry has expired
		if tc.ttl > 0 && time.Since(entry.CompiledAt) > tc.ttl {
			tc.removeElement(elem)
			tc.stats.Misses++
			return nil, false
		}

		// Move to front (most recently used)
		tc.lru.MoveToFront(elem)
		entry.LastUsed = time.Now()
		entry.AccessCount++

		tc.stats.Hits++
		return entry.Value, true
	}

	tc.stats.Misses++
	return nil, false
}

// Set adds or updates a value in the cache
func (tc *TemplateCache) Set(key string, value interface{}) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Check if key already exists
	if elem, ok := tc.cache[key]; ok {
		tc.lru.MoveToFront(elem)
		entry := elem.Value.(*TemplateCacheEntry)
		entry.Value = value
		entry.CompiledAt = time.Now()
		entry.LastUsed = time.Now()
		return
	}

	// Add new entry
	entry := &TemplateCacheEntry{
		Key:         key,
		Value:       value,
		CompiledAt:  time.Now(),
		LastUsed:    time.Now(),
		AccessCount: 0,
	}
	elem := tc.lru.PushFront(entry)
	tc.cache[key] = elem

	// Check if we need to evict
	if tc.lru.Len() > tc.maxSize {
		tc.removeOldest()
	}

	tc.stats.TotalSize = tc.lru.Len()
}

// Delete removes a key from the cache
func (tc *TemplateCache) Delete(key string) bool {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if elem, ok := tc.cache[key]; ok {
		tc.removeElement(elem)
		return true
	}
	return false
}

// Clear removes all entries from the cache
func (tc *TemplateCache) Clear() {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	for _, elem := range tc.cache {
		if tc.evictFunc != nil {
			entry := elem.Value.(*TemplateCacheEntry)
			tc.evictFunc(entry.Key, entry.Value)
		}
	}

	tc.cache = make(map[string]*list.Element)
	tc.lru.Init()
	tc.stats.TotalSize = 0
}

// Stats returns cache statistics
func (tc *TemplateCache) Stats() Stats {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.stats
}

// Purge removes expired entries
func (tc *TemplateCache) Purge() int {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.ttl <= 0 {
		return 0
	}

	var removed int
	now := time.Now()

	// Iterate through the list from back (oldest) to front (newest)
	for elem := tc.lru.Back(); elem != nil; {
		next := elem.Prev()
		entry := elem.Value.(*TemplateCacheEntry)

		if now.Sub(entry.CompiledAt) > tc.ttl {
			tc.removeElement(elem)
			removed++
		} else {
			// Since we're iterating from oldest to newest,
			// once we find a non-expired entry, we can stop
			break
		}

		elem = next
	}

	return removed
}

// StartCleanupTask starts a background task to periodically purge expired entries
func (tc *TemplateCache) StartCleanupTask(interval time.Duration) chan struct{} {
	if interval <= 0 {
		interval = 5 * time.Minute
	}

	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				tc.Purge()
			case <-done:
				return
			}
		}
	}()

	return done
}

// removeOldest removes the oldest entry from the cache
func (tc *TemplateCache) removeOldest() {
	elem := tc.lru.Back()
	if elem != nil {
		tc.removeElement(elem)
	}
}

// removeElement removes an element from the cache
func (tc *TemplateCache) removeElement(elem *list.Element) {
	tc.lru.Remove(elem)
	entry := elem.Value.(*TemplateCacheEntry)
	delete(tc.cache, entry.Key)

	if tc.evictFunc != nil {
		tc.evictFunc(entry.Key, entry.Value)
	}

	tc.stats.Evictions++
	tc.stats.TotalSize = tc.lru.Len()
}
