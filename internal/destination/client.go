package destination

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// HTTPClient is a configured HTTP client with connection pooling
type HTTPClient struct {
	client    *http.Client
	userAgent string
}

// HTTPClientConfig holds configuration for the HTTP client
type HTTPClientConfig struct {
	// Timeout for the entire request
	Timeout time.Duration

	// Maximum idle connections across all hosts
	MaxIdleConns int

	// Maximum idle connections per host
	MaxIdleConnsPerHost int

	// Maximum total connections per host
	MaxConnsPerHost int

	// Idle connection timeout
	IdleConnTimeout time.Duration

	// TLS handshake timeout
	TLSHandshakeTimeout time.Duration

	// User agent string
	UserAgent string

	// Skip TLS verification (not recommended for production)
	InsecureSkipVerify bool
}

// DefaultHTTPClientConfig returns default HTTP client configuration
func DefaultHTTPClientConfig() *HTTPClientConfig {
	return &HTTPClientConfig{
		Timeout:             30 * time.Second,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		MaxConnsPerHost:     10,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
		UserAgent:           "alertmanager-gateway/1.0",
		InsecureSkipVerify:  false,
	}
}

// NewHTTPClient creates a new HTTP client with connection pooling
func NewHTTPClient(config *HTTPClientConfig) *HTTPClient {
	if config == nil {
		config = DefaultHTTPClientConfig()
	}

	// Create transport with connection pooling
	transport := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:        config.MaxIdleConns,
		MaxIdleConnsPerHost: config.MaxIdleConnsPerHost,
		MaxConnsPerHost:     config.MaxConnsPerHost,
		IdleConnTimeout:     config.IdleConnTimeout,
		TLSHandshakeTimeout: config.TLSHandshakeTimeout,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: config.InsecureSkipVerify,
		},
		DisableCompression: false,
		DisableKeepAlives:  false,
	}

	return &HTTPClient{
		client: &http.Client{
			Transport: transport,
			Timeout:   config.Timeout,
		},
		userAgent: config.UserAgent,
	}
}

// Do executes an HTTP request
func (c *HTTPClient) Do(req *http.Request) (*http.Response, error) {
	// Set user agent if not already set
	if req.Header.Get("User-Agent") == "" && c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}

	return c.client.Do(req)
}

// DoWithContext executes an HTTP request with context
func (c *HTTPClient) DoWithContext(ctx context.Context, req *http.Request) (*http.Response, error) {
	req = req.WithContext(ctx)
	return c.Do(req)
}

// Post sends a POST request
func (c *HTTPClient) Post(ctx context.Context, url string, contentType string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	return c.Do(req)
}

// Get sends a GET request
func (c *HTTPClient) Get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	return c.Do(req)
}

// CloseIdleConnections closes any idle connections
func (c *HTTPClient) CloseIdleConnections() {
	if transport, ok := c.client.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

// HTTPResponse wraps the HTTP response with helper methods
type HTTPResponse struct {
	*http.Response
}

// IsSuccess returns true if the status code is 2xx
func (r *HTTPResponse) IsSuccess() bool {
	return r.StatusCode >= 200 && r.StatusCode < 300
}

// ReadBody reads the response body and closes it
func (r *HTTPResponse) ReadBody() ([]byte, error) {
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}

// WrapResponse wraps an http.Response with helper methods
func WrapResponse(resp *http.Response) *HTTPResponse {
	return &HTTPResponse{Response: resp}
}
