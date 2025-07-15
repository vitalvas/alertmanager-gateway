package destination

import (
	"crypto/tls"
	"net"
	"net/http"
	"sync"
	"time"
)

// PoolConfig contains configuration for the HTTP client pool
type PoolConfig struct {
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	MaxConnsPerHost       int
	IdleConnTimeout       time.Duration
	ResponseHeaderTimeout time.Duration
	DialTimeout           time.Duration
	KeepAlive             time.Duration
	TLSHandshakeTimeout   time.Duration
	ExpectContinueTimeout time.Duration
	DisableCompression    bool
	DisableKeepAlives     bool
}

// DefaultPoolConfig returns a default pool configuration optimized for webhook delivery
func DefaultPoolConfig() *PoolConfig {
	return &PoolConfig{
		MaxIdleConns:          100,
		MaxIdleConnsPerHost:   10,
		MaxConnsPerHost:       0, // No limit
		IdleConnTimeout:       90 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DialTimeout:           10 * time.Second,
		KeepAlive:             30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DisableCompression:    false,
		DisableKeepAlives:     false,
	}
}

// ClientPool manages a pool of HTTP clients
type ClientPool struct {
	mu      sync.RWMutex
	clients map[string]*http.Client
	config  *PoolConfig
	stats   PoolStats
}

// PoolStats contains statistics about the client pool
type PoolStats struct {
	ActiveClients int
	TotalRequests int64
	CacheHits     int64
	CacheMisses   int64
}

// NewClientPool creates a new HTTP client pool
func NewClientPool(config *PoolConfig) *ClientPool {
	if config == nil {
		config = DefaultPoolConfig()
	}

	return &ClientPool{
		clients: make(map[string]*http.Client),
		config:  config,
	}
}

// GetClient returns an HTTP client for the given destination
func (p *ClientPool) GetClient(destinationName string, timeout time.Duration) *http.Client {
	p.mu.RLock()
	client, exists := p.clients[destinationName]
	p.mu.RUnlock()

	if exists {
		p.mu.Lock()
		p.stats.CacheHits++
		p.stats.TotalRequests++
		p.mu.Unlock()
		return client
	}

	// Create new client
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock
	if client, exists = p.clients[destinationName]; exists {
		p.stats.CacheHits++
		p.stats.TotalRequests++
		return client
	}

	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   p.config.DialTimeout,
			KeepAlive: p.config.KeepAlive,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          p.config.MaxIdleConns,
		MaxIdleConnsPerHost:   p.config.MaxIdleConnsPerHost,
		MaxConnsPerHost:       p.config.MaxConnsPerHost,
		IdleConnTimeout:       p.config.IdleConnTimeout,
		TLSHandshakeTimeout:   p.config.TLSHandshakeTimeout,
		ExpectContinueTimeout: p.config.ExpectContinueTimeout,
		ResponseHeaderTimeout: p.config.ResponseHeaderTimeout,
		DisableCompression:    p.config.DisableCompression,
		DisableKeepAlives:     p.config.DisableKeepAlives,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	client = &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}

	p.clients[destinationName] = client
	p.stats.ActiveClients = len(p.clients)
	p.stats.CacheMisses++
	p.stats.TotalRequests++

	return client
}

// Stats returns pool statistics
func (p *ClientPool) Stats() PoolStats {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := p.stats
	stats.ActiveClients = len(p.clients)
	return stats
}

// Close closes all idle connections in the pool
func (p *ClientPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, client := range p.clients {
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
}

// RemoveClient removes a specific client from the pool
func (p *ClientPool) RemoveClient(destinationName string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if client, exists := p.clients[destinationName]; exists {
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
		delete(p.clients, destinationName)
		p.stats.ActiveClients = len(p.clients)
	}
}

// CloseIdleConnections closes idle connections for all clients
func (p *ClientPool) CloseIdleConnections() {
	p.mu.RLock()
	defer p.mu.RUnlock()

	for _, client := range p.clients {
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
}

// SharedTransport creates a shared transport with the pool configuration
// This can be used when you want multiple clients to share the same connection pool
func (p *ClientPool) SharedTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   p.config.DialTimeout,
			KeepAlive: p.config.KeepAlive,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          p.config.MaxIdleConns * 2, // Double for shared use
		MaxIdleConnsPerHost:   p.config.MaxIdleConnsPerHost,
		MaxConnsPerHost:       p.config.MaxConnsPerHost,
		IdleConnTimeout:       p.config.IdleConnTimeout,
		TLSHandshakeTimeout:   p.config.TLSHandshakeTimeout,
		ExpectContinueTimeout: p.config.ExpectContinueTimeout,
		ResponseHeaderTimeout: p.config.ResponseHeaderTimeout,
		DisableCompression:    p.config.DisableCompression,
		DisableKeepAlives:     p.config.DisableKeepAlives,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}
}
