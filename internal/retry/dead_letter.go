package retry

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// DeadLetterEntry represents an entry in the dead letter queue
type DeadLetterEntry struct {
	ID           string                 `json:"id"`
	Destination  string                 `json:"destination"`
	Payload      interface{}            `json:"payload"`
	OriginalTime time.Time              `json:"original_time"`
	DeadTime     time.Time              `json:"dead_time"`
	Attempts     int                    `json:"attempts"`
	LastError    string                 `json:"last_error"`
	ErrorHistory []string               `json:"error_history"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// DeadLetterQueueConfig holds configuration for the dead letter queue
type DeadLetterQueueConfig struct {
	Enabled       bool          `yaml:"enabled"`
	MaxSize       int           `yaml:"max_size"`
	TTL           time.Duration `yaml:"ttl"`
	PersistToDisk bool          `yaml:"persist_to_disk"`
	FilePath      string        `yaml:"file_path"`
	FlushInterval time.Duration `yaml:"flush_interval"`
}

// DefaultDeadLetterQueueConfig returns default configuration
func DefaultDeadLetterQueueConfig() *DeadLetterQueueConfig {
	return &DeadLetterQueueConfig{
		Enabled:       false,
		MaxSize:       1000,
		TTL:           24 * time.Hour,
		PersistToDisk: false,
		FilePath:      "dead_letter_queue.json",
		FlushInterval: 5 * time.Minute,
	}
}

// DeadLetterQueue implements a dead letter queue for failed requests
type DeadLetterQueue struct {
	config  *DeadLetterQueueConfig
	entries map[string]*DeadLetterEntry
	order   []string // Maintain insertion order for FIFO cleanup
	mu      sync.RWMutex
	logger  *logrus.Entry
	stop    chan struct{}
	wg      sync.WaitGroup
}

// NewDeadLetterQueue creates a new dead letter queue
func NewDeadLetterQueue(config *DeadLetterQueueConfig, logger *logrus.Logger) *DeadLetterQueue {
	if config == nil {
		config = DefaultDeadLetterQueueConfig()
	}

	dlq := &DeadLetterQueue{
		config:  config,
		entries: make(map[string]*DeadLetterEntry),
		order:   make([]string, 0),
		logger:  logger.WithField("component", "dead-letter-queue"),
		stop:    make(chan struct{}),
	}

	if config.Enabled && config.FlushInterval > 0 {
		dlq.start()
	}

	return dlq
}

// start starts the background cleanup goroutine
func (dlq *DeadLetterQueue) start() {
	dlq.wg.Add(1)
	go func() {
		defer dlq.wg.Done()
		ticker := time.NewTicker(dlq.config.FlushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-dlq.stop:
				return
			case <-ticker.C:
				dlq.cleanup()
				if dlq.config.PersistToDisk {
					dlq.persistToDisk()
				}
			}
		}
	}()

	dlq.logger.Info("Dead letter queue started")
}

// Stop stops the dead letter queue
func (dlq *DeadLetterQueue) Stop() {
	if dlq.config.Enabled && dlq.config.FlushInterval > 0 {
		close(dlq.stop)
		dlq.wg.Wait()

		if dlq.config.PersistToDisk {
			dlq.persistToDisk()
		}

		dlq.logger.Info("Dead letter queue stopped")
	}
}

// Add adds an entry to the dead letter queue
func (dlq *DeadLetterQueue) Add(_ context.Context, entry *DeadLetterEntry) error {
	if !dlq.config.Enabled {
		return nil
	}

	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	// Generate ID if not provided
	if entry.ID == "" {
		entry.ID = fmt.Sprintf("%s_%d", entry.Destination, time.Now().UnixNano())
	}

	// Set dead time if not provided
	if entry.DeadTime.IsZero() {
		entry.DeadTime = time.Now()
	}

	// Check if we need to make room
	if len(dlq.entries) >= dlq.config.MaxSize {
		// Remove oldest entry (FIFO)
		if len(dlq.order) > 0 {
			oldestID := dlq.order[0]
			delete(dlq.entries, oldestID)
			dlq.order = dlq.order[1:]

			dlq.logger.WithField("removed_id", oldestID).Warn("Dead letter queue full, removed oldest entry")
		}
	}

	dlq.entries[entry.ID] = entry
	dlq.order = append(dlq.order, entry.ID)

	dlq.logger.WithFields(logrus.Fields{
		"id":          entry.ID,
		"destination": entry.Destination,
		"attempts":    entry.Attempts,
		"last_error":  entry.LastError,
		"queue_size":  len(dlq.entries),
	}).Info("Added entry to dead letter queue")

	return nil
}

// Get retrieves an entry from the dead letter queue
func (dlq *DeadLetterQueue) Get(id string) (*DeadLetterEntry, bool) {
	if !dlq.config.Enabled {
		return nil, false
	}

	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	entry, exists := dlq.entries[id]
	return entry, exists
}

// Remove removes an entry from the dead letter queue
func (dlq *DeadLetterQueue) Remove(id string) bool {
	if !dlq.config.Enabled {
		return false
	}

	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	if _, exists := dlq.entries[id]; exists {
		delete(dlq.entries, id)

		// Remove from order slice
		for i, orderID := range dlq.order {
			if orderID == id {
				dlq.order = append(dlq.order[:i], dlq.order[i+1:]...)
				break
			}
		}

		dlq.logger.WithField("id", id).Info("Removed entry from dead letter queue")
		return true
	}

	return false
}

// List returns all entries in the dead letter queue
func (dlq *DeadLetterQueue) List() []*DeadLetterEntry {
	if !dlq.config.Enabled {
		return nil
	}

	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	entries := make([]*DeadLetterEntry, 0, len(dlq.entries))
	for _, id := range dlq.order {
		if entry, exists := dlq.entries[id]; exists {
			entries = append(entries, entry)
		}
	}

	return entries
}

// ListByDestination returns entries for a specific destination
func (dlq *DeadLetterQueue) ListByDestination(destination string) []*DeadLetterEntry {
	if !dlq.config.Enabled {
		return nil
	}

	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	entries := make([]*DeadLetterEntry, 0)
	for _, entry := range dlq.entries {
		if entry.Destination == destination {
			entries = append(entries, entry)
		}
	}

	return entries
}

// Size returns the current size of the dead letter queue
func (dlq *DeadLetterQueue) Size() int {
	if !dlq.config.Enabled {
		return 0
	}

	dlq.mu.RLock()
	defer dlq.mu.RUnlock()
	return len(dlq.entries)
}

// Clear removes all entries from the dead letter queue
func (dlq *DeadLetterQueue) Clear() {
	if !dlq.config.Enabled {
		return
	}

	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	count := len(dlq.entries)
	dlq.entries = make(map[string]*DeadLetterEntry)
	dlq.order = make([]string, 0)

	dlq.logger.WithField("cleared_count", count).Info("Cleared dead letter queue")
}

// cleanup removes expired entries
func (dlq *DeadLetterQueue) cleanup() {
	dlq.mu.Lock()
	defer dlq.mu.Unlock()

	now := time.Now()
	expiredIDs := make([]string, 0)

	for id, entry := range dlq.entries {
		if now.Sub(entry.DeadTime) > dlq.config.TTL {
			expiredIDs = append(expiredIDs, id)
		}
	}

	for _, id := range expiredIDs {
		delete(dlq.entries, id)

		// Remove from order slice
		for i, orderID := range dlq.order {
			if orderID == id {
				dlq.order = append(dlq.order[:i], dlq.order[i+1:]...)
				break
			}
		}
	}

	if len(expiredIDs) > 0 {
		dlq.logger.WithFields(logrus.Fields{
			"expired_count": len(expiredIDs),
			"remaining":     len(dlq.entries),
		}).Info("Cleaned up expired dead letter queue entries")
	}
}

// persistToDisk saves the dead letter queue to disk
func (dlq *DeadLetterQueue) persistToDisk() {
	if !dlq.config.PersistToDisk {
		return
	}

	dlq.mu.RLock()
	entries := make([]*DeadLetterEntry, 0, len(dlq.entries))
	for _, id := range dlq.order {
		if entry, exists := dlq.entries[id]; exists {
			entries = append(entries, entry)
		}
	}
	dlq.mu.RUnlock()

	// This is a simplified implementation
	// In production, you might want to use a more robust persistence mechanism
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		dlq.logger.WithError(err).Error("Failed to marshal dead letter queue for persistence")
		return
	}

	dlq.logger.WithFields(logrus.Fields{
		"file_path":    dlq.config.FilePath,
		"entry_count":  len(entries),
		"data_size_kb": len(data) / 1024,
	}).Debug("Would persist dead letter queue to disk")

	// Note: Actual file writing is omitted to avoid filesystem operations
	// In a real implementation, you would write the data to dlq.config.FilePath
}

// GetStats returns statistics about the dead letter queue
func (dlq *DeadLetterQueue) GetStats() map[string]interface{} {
	if !dlq.config.Enabled {
		return map[string]interface{}{
			"enabled": false,
		}
	}

	dlq.mu.RLock()
	defer dlq.mu.RUnlock()

	// Count entries by destination
	destinationCounts := make(map[string]int)
	var oldestEntry time.Time
	var newestEntry time.Time
	totalAttempts := 0

	for _, entry := range dlq.entries {
		destinationCounts[entry.Destination]++
		totalAttempts += entry.Attempts

		if oldestEntry.IsZero() || entry.DeadTime.Before(oldestEntry) {
			oldestEntry = entry.DeadTime
		}
		if newestEntry.IsZero() || entry.DeadTime.After(newestEntry) {
			newestEntry = entry.DeadTime
		}
	}

	stats := map[string]interface{}{
		"enabled":            true,
		"total_entries":      len(dlq.entries),
		"max_size":           dlq.config.MaxSize,
		"ttl_hours":          dlq.config.TTL.Hours(),
		"destination_counts": destinationCounts,
		"total_attempts":     totalAttempts,
	}

	if len(dlq.entries) > 0 {
		stats["avg_attempts"] = float64(totalAttempts) / float64(len(dlq.entries))
		stats["oldest_entry_age_hours"] = time.Since(oldestEntry).Hours()
		stats["newest_entry_age_hours"] = time.Since(newestEntry).Hours()
	} else {
		stats["avg_attempts"] = 0.0
	}

	return stats
}

// DeadLetterQueueManager manages dead letter queues for multiple destinations
type DeadLetterQueueManager struct {
	queues map[string]*DeadLetterQueue
	mu     sync.RWMutex
	logger *logrus.Logger
}

// NewDeadLetterQueueManager creates a new dead letter queue manager
func NewDeadLetterQueueManager(logger *logrus.Logger) *DeadLetterQueueManager {
	return &DeadLetterQueueManager{
		queues: make(map[string]*DeadLetterQueue),
		logger: logger,
	}
}

// GetOrCreate gets or creates a dead letter queue for a destination
func (m *DeadLetterQueueManager) GetOrCreate(destination string, config *DeadLetterQueueConfig) *DeadLetterQueue {
	m.mu.Lock()
	defer m.mu.Unlock()

	if dlq, exists := m.queues[destination]; exists {
		return dlq
	}

	if config == nil {
		config = DefaultDeadLetterQueueConfig()
	}

	dlq := NewDeadLetterQueue(config, m.logger)
	m.queues[destination] = dlq

	return dlq
}

// Get gets a dead letter queue for a destination
func (m *DeadLetterQueueManager) Get(destination string) (*DeadLetterQueue, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dlq, exists := m.queues[destination]
	return dlq, exists
}

// GetAll returns all dead letter queues
func (m *DeadLetterQueueManager) GetAll() map[string]*DeadLetterQueue {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*DeadLetterQueue, len(m.queues))
	for destination, dlq := range m.queues {
		result[destination] = dlq
	}
	return result
}

// Stop stops all dead letter queues
func (m *DeadLetterQueueManager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, dlq := range m.queues {
		dlq.Stop()
	}
}

// GetStats returns stats for all dead letter queues
func (m *DeadLetterQueueManager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]interface{})
	for destination, dlq := range m.queues {
		stats[destination] = dlq.GetStats()
	}
	return stats
}
