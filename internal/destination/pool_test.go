package destination

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultPoolConfig(t *testing.T) {
	config := DefaultPoolConfig()

	assert.Equal(t, 100, config.MaxIdleConns)
	assert.Equal(t, 10, config.MaxIdleConnsPerHost)
	assert.Equal(t, 0, config.MaxConnsPerHost)
	assert.Equal(t, 90*time.Second, config.IdleConnTimeout)
	assert.Equal(t, 30*time.Second, config.ResponseHeaderTimeout)
	assert.Equal(t, 10*time.Second, config.DialTimeout)
	assert.Equal(t, 30*time.Second, config.KeepAlive)
	assert.Equal(t, 10*time.Second, config.TLSHandshakeTimeout)
	assert.Equal(t, 1*time.Second, config.ExpectContinueTimeout)
	assert.False(t, config.DisableCompression)
	assert.False(t, config.DisableKeepAlives)
}

func TestNewClientPool(t *testing.T) {
	t.Run("with config", func(t *testing.T) {
		config := &PoolConfig{
			MaxIdleConns: 50,
			DialTimeout:  5 * time.Second,
		}

		pool := NewClientPool(config)

		assert.NotNil(t, pool)
		assert.Equal(t, config, pool.config)
		assert.NotNil(t, pool.clients)
		assert.Empty(t, pool.clients)
	})

	t.Run("with nil config", func(t *testing.T) {
		pool := NewClientPool(nil)

		assert.NotNil(t, pool)
		assert.NotNil(t, pool.config)
		assert.Equal(t, DefaultPoolConfig(), pool.config)
	})
}

func TestClientPool_GetClient(t *testing.T) {
	timeout := 30 * time.Second

	t.Run("first request creates new client", func(t *testing.T) {
		pool := NewClientPool(nil)
		client := pool.GetClient("test-dest", timeout)

		assert.NotNil(t, client)
		assert.Equal(t, timeout, client.Timeout)

		stats := pool.Stats()
		assert.Equal(t, 1, stats.ActiveClients)
		assert.Equal(t, int64(1), stats.TotalRequests)
		assert.Equal(t, int64(0), stats.CacheHits)
		assert.Equal(t, int64(1), stats.CacheMisses)
	})

	t.Run("second request returns cached client", func(t *testing.T) {
		pool := NewClientPool(nil)
		client1 := pool.GetClient("test-dest", timeout)
		client2 := pool.GetClient("test-dest", timeout)

		assert.Same(t, client1, client2)

		stats := pool.Stats()
		assert.Equal(t, 1, stats.ActiveClients)
		assert.Equal(t, int64(2), stats.TotalRequests)
		assert.Equal(t, int64(1), stats.CacheHits)
		assert.Equal(t, int64(1), stats.CacheMisses)
	})

	t.Run("different destinations get different clients", func(t *testing.T) {
		pool := NewClientPool(nil)
		client1 := pool.GetClient("dest1", timeout)
		client2 := pool.GetClient("dest2", timeout)

		assert.NotSame(t, client1, client2)

		stats := pool.Stats()
		assert.Equal(t, 2, stats.ActiveClients)
	})
}

func TestClientPool_RemoveClient(t *testing.T) {
	pool := NewClientPool(nil)
	timeout := 30 * time.Second

	// Create a client
	client := pool.GetClient("test-dest", timeout)
	assert.NotNil(t, client)

	stats := pool.Stats()
	assert.Equal(t, 1, stats.ActiveClients)

	// Remove the client
	pool.RemoveClient("test-dest")

	stats = pool.Stats()
	assert.Equal(t, 0, stats.ActiveClients)

	// Removing non-existent client should not panic
	pool.RemoveClient("non-existent")
}

func TestClientPool_Close(t *testing.T) {
	pool := NewClientPool(nil)
	timeout := 30 * time.Second

	// Create some clients
	pool.GetClient("dest1", timeout)
	pool.GetClient("dest2", timeout)

	// Close should not panic
	assert.NotPanics(t, func() {
		pool.Close()
	})
}

func TestClientPool_CloseIdleConnections(t *testing.T) {
	pool := NewClientPool(nil)
	timeout := 30 * time.Second

	// Create some clients
	pool.GetClient("dest1", timeout)
	pool.GetClient("dest2", timeout)

	// CloseIdleConnections should not panic
	assert.NotPanics(t, func() {
		pool.CloseIdleConnections()
	})
}

func TestClientPool_SharedTransport(t *testing.T) {
	config := &PoolConfig{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 5,
		DialTimeout:         5 * time.Second,
		KeepAlive:           15 * time.Second,
	}

	pool := NewClientPool(config)
	transport := pool.SharedTransport()

	assert.NotNil(t, transport)
	assert.Equal(t, config.MaxIdleConns*2, transport.MaxIdleConns) // Doubled for shared use
	assert.Equal(t, config.MaxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
}

func TestClientPool_Stats(t *testing.T) {
	pool := NewClientPool(nil)
	timeout := 30 * time.Second

	// Initial stats
	stats := pool.Stats()
	assert.Equal(t, 0, stats.ActiveClients)
	assert.Equal(t, int64(0), stats.TotalRequests)
	assert.Equal(t, int64(0), stats.CacheHits)
	assert.Equal(t, int64(0), stats.CacheMisses)

	// Create first client
	pool.GetClient("dest1", timeout)
	stats = pool.Stats()
	assert.Equal(t, 1, stats.ActiveClients)
	assert.Equal(t, int64(1), stats.TotalRequests)
	assert.Equal(t, int64(0), stats.CacheHits)
	assert.Equal(t, int64(1), stats.CacheMisses)

	// Get same client again
	pool.GetClient("dest1", timeout)
	stats = pool.Stats()
	assert.Equal(t, 1, stats.ActiveClients)
	assert.Equal(t, int64(2), stats.TotalRequests)
	assert.Equal(t, int64(1), stats.CacheHits)
	assert.Equal(t, int64(1), stats.CacheMisses)

	// Create second client
	pool.GetClient("dest2", timeout)
	stats = pool.Stats()
	assert.Equal(t, 2, stats.ActiveClients)
	assert.Equal(t, int64(3), stats.TotalRequests)
	assert.Equal(t, int64(1), stats.CacheHits)
	assert.Equal(t, int64(2), stats.CacheMisses)
}

func TestClientPool_ConcurrentAccess(t *testing.T) {
	pool := NewClientPool(nil)
	timeout := 30 * time.Second

	// Test concurrent access to the same destination
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- true }()
			client := pool.GetClient("test-dest", timeout)
			assert.NotNil(t, client)
		}()
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	stats := pool.Stats()
	assert.Equal(t, 1, stats.ActiveClients) // Should only have one client for the destination
	assert.Equal(t, int64(10), stats.TotalRequests)
}

func TestClientPool_Transport_Configuration(t *testing.T) {
	config := &PoolConfig{
		MaxIdleConns:        50,
		MaxIdleConnsPerHost: 5,
		MaxConnsPerHost:     10,
		IdleConnTimeout:     60 * time.Second,
		DialTimeout:         5 * time.Second,
		KeepAlive:           15 * time.Second,
		TLSHandshakeTimeout: 5 * time.Second,
		DisableCompression:  true,
		DisableKeepAlives:   true,
	}

	pool := NewClientPool(config)
	client := pool.GetClient("test", 30*time.Second)

	require.NotNil(t, client)
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)

	assert.Equal(t, config.MaxIdleConns, transport.MaxIdleConns)
	assert.Equal(t, config.MaxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
	assert.Equal(t, config.MaxConnsPerHost, transport.MaxConnsPerHost)
	assert.Equal(t, config.IdleConnTimeout, transport.IdleConnTimeout)
	assert.Equal(t, config.TLSHandshakeTimeout, transport.TLSHandshakeTimeout)
	assert.Equal(t, config.DisableCompression, transport.DisableCompression)
	assert.Equal(t, config.DisableKeepAlives, transport.DisableKeepAlives)
}
