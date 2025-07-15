package destination

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultHTTPClientConfig(t *testing.T) {
	config := DefaultHTTPClientConfig()

	assert.Equal(t, 30*time.Second, config.Timeout)
	assert.Equal(t, 100, config.MaxIdleConns)
	assert.Equal(t, 10, config.MaxIdleConnsPerHost)
	assert.Equal(t, 10, config.MaxConnsPerHost)
	assert.Equal(t, 90*time.Second, config.IdleConnTimeout)
	assert.Equal(t, 10*time.Second, config.TLSHandshakeTimeout)
	assert.Equal(t, "alertmanager-gateway/1.0", config.UserAgent)
	assert.False(t, config.InsecureSkipVerify)
}

func TestNewHTTPClient(t *testing.T) {
	// Test with default config
	client := NewHTTPClient(nil)
	assert.NotNil(t, client)
	assert.NotNil(t, client.client)
	assert.Equal(t, "alertmanager-gateway/1.0", client.userAgent)

	// Test with custom config
	config := &HTTPClientConfig{
		Timeout:   5 * time.Second,
		UserAgent: "test-agent",
	}
	client2 := NewHTTPClient(config)
	assert.NotNil(t, client2)
	assert.Equal(t, "test-agent", client2.userAgent)
}

func TestHTTPClient_Do(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "test-agent", r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	}))
	defer server.Close()

	client := NewHTTPClient(&HTTPClientConfig{
		UserAgent: "test-agent",
		Timeout:   5 * time.Second,
	})

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, "success", string(body))
}

func TestHTTPClient_DoWithContext(t *testing.T) {
	// Create test server with delay
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClient(nil)

	// Test with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req, err := http.NewRequest(http.MethodGet, server.URL, nil)
	require.NoError(t, err)

	_, err = client.DoWithContext(ctx, req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "context canceled")
}

func TestHTTPClient_Post(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, `{"test":"data"}`, string(body))

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewHTTPClient(nil)

	resp, err := client.Post(
		context.Background(),
		server.URL,
		"application/json",
		strings.NewReader(`{"test":"data"}`),
	)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusCreated, resp.StatusCode)
}

func TestHTTPClient_Get(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("get response"))
	}))
	defer server.Close()

	client := NewHTTPClient(nil)

	resp, err := client.Get(context.Background(), server.URL)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Equal(t, "get response", string(body))
}

func TestHTTPClient_CloseIdleConnections(_ *testing.T) {
	client := NewHTTPClient(nil)

	// Should not panic
	client.CloseIdleConnections()
}

func TestHTTPResponse_IsSuccess(t *testing.T) {
	tests := []struct {
		statusCode int
		isSuccess  bool
	}{
		{200, true},
		{201, true},
		{204, true},
		{299, true},
		{300, false},
		{400, false},
		{500, false},
	}

	for _, tt := range tests {
		resp := &HTTPResponse{
			Response: &http.Response{
				StatusCode: tt.statusCode,
			},
		}
		assert.Equal(t, tt.isSuccess, resp.IsSuccess(), "StatusCode: %d", tt.statusCode)
	}
}

func TestHTTPResponse_ReadBody(t *testing.T) {
	resp := &HTTPResponse{
		Response: &http.Response{
			Body: io.NopCloser(strings.NewReader("test body")),
		},
	}

	body, err := resp.ReadBody()
	require.NoError(t, err)
	assert.Equal(t, "test body", string(body))
}

func TestWrapResponse(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
	}

	wrapped := WrapResponse(resp)
	assert.NotNil(t, wrapped)
	assert.Equal(t, 200, wrapped.StatusCode)
	assert.True(t, wrapped.IsSuccess())
}
