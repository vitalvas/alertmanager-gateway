package retry

import (
	"context"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

func TestNewDeadLetterQueue(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Test with nil config
	dlq := NewDeadLetterQueue(nil, logger)
	assert.NotNil(t, dlq)
	assert.NotNil(t, dlq.config)
	assert.False(t, dlq.config.Enabled) // Default is disabled

	// Test with custom config
	config := &DeadLetterQueueConfig{
		Enabled: true,
		MaxSize: 100,
		TTL:     time.Hour,
	}
	dlq = NewDeadLetterQueue(config, logger)
	assert.True(t, dlq.config.Enabled)
	assert.Equal(t, 100, dlq.config.MaxSize)
	assert.Equal(t, time.Hour, dlq.config.TTL)

	dlq.Stop() // Clean up
}

func TestDeadLetterQueue_AddAndGet(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &DeadLetterQueueConfig{
		Enabled: true,
		MaxSize: 10,
		TTL:     time.Hour,
	}
	dlq := NewDeadLetterQueue(config, logger)
	defer dlq.Stop()

	// Create test entry
	entry := &DeadLetterEntry{
		ID:           "test-1",
		Destination:  "test-dest",
		Payload:      map[string]string{"alert": "test"},
		OriginalTime: time.Now().Add(-time.Hour),
		Attempts:     3,
		LastError:    "Connection refused",
		ErrorHistory: []string{"timeout", "connection refused", "connection refused"},
		Metadata:     map[string]interface{}{"retry_count": 3},
	}

	// Add entry
	err := dlq.Add(context.Background(), entry)
	assert.NoError(t, err)
	assert.Equal(t, 1, dlq.Size())

	// Get entry
	retrieved, exists := dlq.Get("test-1")
	assert.True(t, exists)
	assert.Equal(t, "test-1", retrieved.ID)
	assert.Equal(t, "test-dest", retrieved.Destination)
	assert.Equal(t, 3, retrieved.Attempts)
}

func TestDeadLetterQueue_DisabledOperations(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &DeadLetterQueueConfig{
		Enabled: false,
	}
	dlq := NewDeadLetterQueue(config, logger)

	entry := &DeadLetterEntry{
		ID:          "test-1",
		Destination: "test-dest",
		Payload:     "test",
	}

	// Operations should not fail but should not do anything
	err := dlq.Add(context.Background(), entry)
	assert.NoError(t, err)

	_, exists := dlq.Get("test-1")
	assert.False(t, exists)

	assert.Equal(t, 0, dlq.Size())

	list := dlq.List()
	assert.Nil(t, list)
}

func TestDeadLetterQueue_MaxSizeLimit(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &DeadLetterQueueConfig{
		Enabled: true,
		MaxSize: 3,
		TTL:     time.Hour,
	}
	dlq := NewDeadLetterQueue(config, logger)
	defer dlq.Stop()

	// Add entries up to max size
	for i := 1; i <= 3; i++ {
		entry := &DeadLetterEntry{
			ID:          string(rune('0' + i)),
			Destination: "test-dest",
			Payload:     i,
		}
		err := dlq.Add(context.Background(), entry)
		assert.NoError(t, err)
	}
	assert.Equal(t, 3, dlq.Size())

	// Add one more entry - should remove oldest
	entry := &DeadLetterEntry{
		ID:          "4",
		Destination: "test-dest",
		Payload:     4,
	}
	err := dlq.Add(context.Background(), entry)
	assert.NoError(t, err)
	assert.Equal(t, 3, dlq.Size())

	// First entry should be removed, last entry should be present
	_, exists := dlq.Get("1")
	assert.False(t, exists)

	_, exists = dlq.Get("4")
	assert.True(t, exists)
}

func TestDeadLetterQueue_Remove(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &DeadLetterQueueConfig{
		Enabled: true,
		MaxSize: 10,
		TTL:     time.Hour,
	}
	dlq := NewDeadLetterQueue(config, logger)
	defer dlq.Stop()

	// Add entry
	entry := &DeadLetterEntry{
		ID:          "test-1",
		Destination: "test-dest",
		Payload:     "test",
	}
	dlq.Add(context.Background(), entry)

	// Remove entry
	removed := dlq.Remove("test-1")
	assert.True(t, removed)
	assert.Equal(t, 0, dlq.Size())

	// Try to remove non-existent entry
	removed = dlq.Remove("nonexistent")
	assert.False(t, removed)
}

func TestDeadLetterQueue_List(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &DeadLetterQueueConfig{
		Enabled: true,
		MaxSize: 10,
		TTL:     time.Hour,
	}
	dlq := NewDeadLetterQueue(config, logger)
	defer dlq.Stop()

	// Add multiple entries
	destinations := []string{"dest1", "dest2", "dest1"}
	for i, dest := range destinations {
		entry := &DeadLetterEntry{
			ID:          string(rune('1' + i)),
			Destination: dest,
			Payload:     i + 1,
		}
		dlq.Add(context.Background(), entry)
	}

	// Test List
	entries := dlq.List()
	assert.Len(t, entries, 3)

	// Entries should be in insertion order
	assert.Equal(t, "1", entries[0].ID)
	assert.Equal(t, "2", entries[1].ID)
	assert.Equal(t, "3", entries[2].ID)

	// Test ListByDestination
	dest1Entries := dlq.ListByDestination("dest1")
	assert.Len(t, dest1Entries, 2)
	for _, entry := range dest1Entries {
		assert.Equal(t, "dest1", entry.Destination)
	}

	dest2Entries := dlq.ListByDestination("dest2")
	assert.Len(t, dest2Entries, 1)
	assert.Equal(t, "dest2", dest2Entries[0].Destination)

	nonexistentEntries := dlq.ListByDestination("nonexistent")
	assert.Len(t, nonexistentEntries, 0)
}

func TestDeadLetterQueue_Clear(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &DeadLetterQueueConfig{
		Enabled: true,
		MaxSize: 10,
		TTL:     time.Hour,
	}
	dlq := NewDeadLetterQueue(config, logger)
	defer dlq.Stop()

	// Add entries
	for i := 1; i <= 5; i++ {
		entry := &DeadLetterEntry{
			ID:          string(rune('0' + i)),
			Destination: "test-dest",
			Payload:     i,
		}
		dlq.Add(context.Background(), entry)
	}
	assert.Equal(t, 5, dlq.Size())

	// Clear queue
	dlq.Clear()
	assert.Equal(t, 0, dlq.Size())

	entries := dlq.List()
	assert.Len(t, entries, 0)
}

func TestDeadLetterQueue_TTLCleanup(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &DeadLetterQueueConfig{
		Enabled:       true,
		MaxSize:       10,
		TTL:           50 * time.Millisecond,
		FlushInterval: 25 * time.Millisecond,
	}
	dlq := NewDeadLetterQueue(config, logger)
	defer dlq.Stop()

	// Add entry with old dead time
	oldEntry := &DeadLetterEntry{
		ID:          "old",
		Destination: "test-dest",
		Payload:     "old",
		DeadTime:    time.Now().Add(-100 * time.Millisecond), // Expired
	}
	dlq.Add(context.Background(), oldEntry)

	// Add entry with recent dead time
	newEntry := &DeadLetterEntry{
		ID:          "new",
		Destination: "test-dest",
		Payload:     "new",
		DeadTime:    time.Now(), // Not expired
	}
	dlq.Add(context.Background(), newEntry)

	assert.Equal(t, 2, dlq.Size())

	// Manually trigger cleanup instead of waiting for timer
	dlq.cleanup()

	// Old entry should be cleaned up, new entry should remain
	assert.Equal(t, 1, dlq.Size())

	_, exists := dlq.Get("old")
	assert.False(t, exists)

	_, exists = dlq.Get("new")
	assert.True(t, exists)
}

func TestDeadLetterQueue_AutoGenerateID(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &DeadLetterQueueConfig{
		Enabled: true,
		MaxSize: 10,
		TTL:     time.Hour,
	}
	dlq := NewDeadLetterQueue(config, logger)
	defer dlq.Stop()

	// Add entry without ID
	entry := &DeadLetterEntry{
		Destination: "test-dest",
		Payload:     "test",
	}
	dlq.Add(context.Background(), entry)

	// ID should be auto-generated
	assert.NotEmpty(t, entry.ID)
	assert.Contains(t, entry.ID, "test-dest")

	// Entry should be retrievable by generated ID
	retrieved, exists := dlq.Get(entry.ID)
	assert.True(t, exists)
	assert.Equal(t, entry.ID, retrieved.ID)
}

func TestDeadLetterQueue_GetStats(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	// Test disabled queue
	disabledConfig := &DeadLetterQueueConfig{Enabled: false}
	disabledDLQ := NewDeadLetterQueue(disabledConfig, logger)
	stats := disabledDLQ.GetStats()
	assert.False(t, stats["enabled"].(bool))

	// Test enabled queue
	config := &DeadLetterQueueConfig{
		Enabled: true,
		MaxSize: 10,
		TTL:     time.Hour,
	}
	dlq := NewDeadLetterQueue(config, logger)
	defer dlq.Stop()

	// Add entries with different destinations and attempts
	entries := []*DeadLetterEntry{
		{
			ID:          "1",
			Destination: "dest1",
			Payload:     "test1",
			DeadTime:    time.Now().Add(-time.Hour),
			Attempts:    3,
		},
		{
			ID:          "2",
			Destination: "dest2",
			Payload:     "test2",
			DeadTime:    time.Now().Add(-30 * time.Minute),
			Attempts:    5,
		},
		{
			ID:          "3",
			Destination: "dest1",
			Payload:     "test3",
			DeadTime:    time.Now(),
			Attempts:    2,
		},
	}

	for _, entry := range entries {
		dlq.Add(context.Background(), entry)
	}

	stats = dlq.GetStats()

	assert.True(t, stats["enabled"].(bool))
	assert.Equal(t, 3, stats["total_entries"])
	assert.Equal(t, 10, stats["max_size"])
	assert.Equal(t, 1.0, stats["ttl_hours"])
	assert.Equal(t, 10, stats["total_attempts"]) // 3 + 5 + 2
	assert.Equal(t, float64(10)/3, stats["avg_attempts"])

	destinationCounts := stats["destination_counts"].(map[string]int)
	assert.Equal(t, 2, destinationCounts["dest1"])
	assert.Equal(t, 1, destinationCounts["dest2"])

	// Age-related stats should be present
	assert.Contains(t, stats, "oldest_entry_age_hours")
	assert.Contains(t, stats, "newest_entry_age_hours")
}

func TestDeadLetterQueueManager(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	manager := NewDeadLetterQueueManager(logger)
	assert.NotNil(t, manager)

	// Test GetOrCreate
	config := &DeadLetterQueueConfig{
		Enabled: true,
		MaxSize: 5,
	}
	dlq1 := manager.GetOrCreate("dest1", config)
	assert.NotNil(t, dlq1)

	// Get the same queue again
	dlq2 := manager.GetOrCreate("dest1", nil)
	assert.Same(t, dlq1, dlq2)

	// Create a different queue
	dlq3 := manager.GetOrCreate("dest2", nil)
	assert.NotSame(t, dlq1, dlq3)

	// Test Get
	dlq4, exists := manager.Get("dest1")
	assert.True(t, exists)
	assert.Same(t, dlq1, dlq4)

	dlq5, exists := manager.Get("nonexistent")
	assert.False(t, exists)
	assert.Nil(t, dlq5)

	// Test GetAll
	all := manager.GetAll()
	assert.Len(t, all, 2)
	assert.Contains(t, all, "dest1")
	assert.Contains(t, all, "dest2")

	// Add some entries for stats testing
	entry := &DeadLetterEntry{
		ID:          "test",
		Destination: "dest1",
		Payload:     "test",
	}
	dlq1.Add(context.Background(), entry)

	// Test GetStats
	stats := manager.GetStats()
	assert.Len(t, stats, 2)
	assert.Contains(t, stats, "dest1")
	assert.Contains(t, stats, "dest2")

	dest1Stats := stats["dest1"].(map[string]interface{})
	assert.Equal(t, 1, dest1Stats["total_entries"])

	// Test Stop
	manager.Stop()
}

func TestDefaultDeadLetterQueueConfig(t *testing.T) {
	config := DefaultDeadLetterQueueConfig()

	assert.False(t, config.Enabled)
	assert.Equal(t, 1000, config.MaxSize)
	assert.Equal(t, 24*time.Hour, config.TTL)
	assert.False(t, config.PersistToDisk)
	assert.Equal(t, "dead_letter_queue.json", config.FilePath)
	assert.Equal(t, 5*time.Minute, config.FlushInterval)
}

func TestDeadLetterQueue_PersistToDisk(t *testing.T) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &DeadLetterQueueConfig{
		Enabled:       true,
		MaxSize:       10,
		TTL:           time.Hour,
		PersistToDisk: true,
		FilePath:      "test_dlq.json",
		FlushInterval: 50 * time.Millisecond,
	}
	dlq := NewDeadLetterQueue(config, logger)
	defer dlq.Stop()

	// Add entry
	entry := &DeadLetterEntry{
		ID:          "test",
		Destination: "test-dest",
		Payload:     "test",
	}
	dlq.Add(context.Background(), entry)

	// Wait for persistence to be triggered
	time.Sleep(100 * time.Millisecond)

	// Note: We don't actually write to disk in the test implementation
	// but the persistToDisk function should be called
	assert.Equal(t, 1, dlq.Size())
}

func BenchmarkDeadLetterQueue_Add(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &DeadLetterQueueConfig{
		Enabled: true,
		MaxSize: 10000,
		TTL:     time.Hour,
	}
	dlq := NewDeadLetterQueue(config, logger)
	defer dlq.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entry := &DeadLetterEntry{
			ID:          string(rune(i)),
			Destination: "bench-dest",
			Payload:     i,
		}
		dlq.Add(context.Background(), entry)
	}
}

func BenchmarkDeadLetterQueue_Get(b *testing.B) {
	logger := logrus.New()
	logger.SetLevel(logrus.ErrorLevel)

	config := &DeadLetterQueueConfig{
		Enabled: true,
		MaxSize: 10000,
		TTL:     time.Hour,
	}
	dlq := NewDeadLetterQueue(config, logger)
	defer dlq.Stop()

	// Pre-populate queue
	for i := 0; i < 1000; i++ {
		entry := &DeadLetterEntry{
			ID:          string(rune(i)),
			Destination: "bench-dest",
			Payload:     i,
		}
		dlq.Add(context.Background(), entry)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dlq.Get(string(rune(i % 1000)))
	}
}
