package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	
	configContent := `
server:
  host: "127.0.0.1"
  port: 8090
  read_timeout: 45s
  write_timeout: 45s
  auth:
    enabled: true
    username: "test"
    password: "secret"

destinations:
  - name: "test-dest"
    path: "/webhook/test"
    method: "POST"
    url: "https://example.com/webhook"
    format: "json"
    engine: "jq"
    transform: |
      {
        message: .message
      }
`
	
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)
	
	// Load config
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)
	require.NotNil(t, cfg)
	
	// Verify server config
	assert.Equal(t, "127.0.0.1", cfg.Server.Host)
	assert.Equal(t, 8090, cfg.Server.Port)
	assert.Equal(t, 45*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 45*time.Second, cfg.Server.WriteTimeout)
	assert.True(t, cfg.Server.Auth.Enabled)
	assert.Equal(t, "test", cfg.Server.Auth.Username)
	assert.Equal(t, "secret", cfg.Server.Auth.Password)
	
	// Verify destination config
	require.Len(t, cfg.Destinations, 1)
	dest := cfg.Destinations[0]
	assert.Equal(t, "test-dest", dest.Name)
	assert.Equal(t, "/webhook/test", dest.Path)
	assert.Equal(t, "POST", dest.Method)
	assert.Equal(t, "https://example.com/webhook", dest.URL)
	assert.Equal(t, "json", dest.Format)
	assert.Equal(t, "jq", dest.Engine)
	assert.Contains(t, dest.Transform, "message: .message")
	assert.True(t, dest.Enabled)
}

func TestLoadConfigWithEnvVars(t *testing.T) {
	// Set environment variables
	os.Setenv("TEST_HOST", "192.168.1.1")
	os.Setenv("TEST_PORT", "9090")
	os.Setenv("TEST_PASSWORD", "env-secret")
	os.Setenv("WEBHOOK_URL", "https://env.example.com/hook")
	defer func() {
		os.Unsetenv("TEST_HOST")
		os.Unsetenv("TEST_PORT")
		os.Unsetenv("TEST_PASSWORD")
		os.Unsetenv("WEBHOOK_URL")
	}()
	
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	
	configContent := `
server:
  host: "${TEST_HOST}"
  port: ${TEST_PORT}
  auth:
    enabled: true
    username: "admin"
    password: "$TEST_PASSWORD"

destinations:
  - name: "env-test"
    path: "/webhook/env"
    url: "${WEBHOOK_URL}"
    format: "json"
    template: '{"test": "data"}'
    headers:
      Authorization: "Bearer ${API_TOKEN:-default-token}"
`
	
	err := os.WriteFile(configPath, []byte(configContent), 0644)
	require.NoError(t, err)
	
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)
	
	assert.Equal(t, "192.168.1.1", cfg.Server.Host)
	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, "env-secret", cfg.Server.Auth.Password)
	assert.Equal(t, "https://env.example.com/hook", cfg.Destinations[0].URL)
	assert.Equal(t, "Bearer default-token", cfg.Destinations[0].Headers["Authorization"])
}

func TestConfigDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	
	minimalConfig := `
destinations:
  - name: "minimal"
    path: "/webhook/minimal"
    url: "https://example.com"
    template: '{"message": "{{ .Status }}"}'
`
	
	err := os.WriteFile(configPath, []byte(minimalConfig), 0644)
	require.NoError(t, err)
	
	cfg, err := LoadConfig(configPath)
	require.NoError(t, err)
	
	// Server defaults
	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 30*time.Second, cfg.Server.ReadTimeout)
	assert.Equal(t, 30*time.Second, cfg.Server.WriteTimeout)
	
	// Destination defaults
	dest := cfg.Destinations[0]
	assert.Equal(t, "POST", dest.Method)
	assert.Equal(t, "json", dest.Format)
	assert.Equal(t, "go-template", dest.Engine)
	assert.True(t, dest.Enabled)
	assert.Equal(t, 1, dest.ParallelRequests)
	assert.Equal(t, 3, dest.Retry.MaxAttempts)
	assert.Equal(t, "exponential", dest.Retry.Backoff)
}

func TestConfigValidation(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		expectError string
	}{
		{
			name: "invalid port",
			config: `
server:
  port: 99999
destinations:
  - name: "test"
    path: "/test"
    url: "https://example.com"
`,
			expectError: "invalid server port",
		},
		{
			name: "missing auth credentials",
			config: `
server:
  auth:
    enabled: true
destinations:
  - name: "test"
    path: "/test"
    url: "https://example.com"
`,
			expectError: "auth enabled but username or password not provided",
		},
		{
			name: "no destinations",
			config: `
server:
  port: 8080
`,
			expectError: "no destinations configured",
		},
		{
			name: "missing destination name",
			config: `
destinations:
  - path: "/test"
    url: "https://example.com"
`,
			expectError: "name is required",
		},
		{
			name: "duplicate destination name",
			config: `
destinations:
  - name: "test"
    path: "/test1"
    url: "https://example.com"
    template: "test"
  - name: "test"
    path: "/test2"
    url: "https://example.com"
    template: "test"
`,
			expectError: "duplicate destination name",
		},
		{
			name: "missing path",
			config: `
destinations:
  - name: "test"
    url: "https://example.com"
`,
			expectError: "path is required",
		},
		{
			name: "invalid path",
			config: `
destinations:
  - name: "test"
    path: "test"
    url: "https://example.com"
`,
			expectError: "path must start with /",
		},
		{
			name: "duplicate path",
			config: `
destinations:
  - name: "test1"
    path: "/test"
    url: "https://example.com"
    template: "test"
  - name: "test2"
    path: "/test"
    url: "https://example.com"
    template: "test"
`,
			expectError: "duplicate destination path",
		},
		{
			name: "missing url",
			config: `
destinations:
  - name: "test"
    path: "/test"
`,
			expectError: "url is required",
		},
		{
			name: "invalid method",
			config: `
destinations:
  - name: "test"
    path: "/test"
    url: "https://example.com"
    method: "INVALID"
`,
			expectError: "invalid method",
		},
		{
			name: "invalid format",
			config: `
destinations:
  - name: "test"
    path: "/test"
    url: "https://example.com"
    format: "xml"
`,
			expectError: "invalid format",
		},
		{
			name: "invalid engine",
			config: `
destinations:
  - name: "test"
    path: "/test"
    url: "https://example.com"
    engine: "invalid"
`,
			expectError: "invalid engine",
		},
		{
			name: "missing template for go-template",
			config: `
destinations:
  - name: "test"
    path: "/test"
    url: "https://example.com"
    engine: "go-template"
`,
			expectError: "template is required for go-template engine",
		},
		{
			name: "missing transform for jq",
			config: `
destinations:
  - name: "test"
    path: "/test"
    url: "https://example.com"
    engine: "jq"
`,
			expectError: "transform is required for jq engine",
		},
		{
			name: "invalid backoff",
			config: `
destinations:
  - name: "test"
    path: "/test"
    url: "https://example.com"
    template: "test"
    retry:
      backoff: "invalid"
`,
			expectError: "invalid retry backoff",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			
			err := os.WriteFile(configPath, []byte(tt.config), 0644)
			require.NoError(t, err)
			
			_, err = LoadConfig(configPath)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.expectError)
		})
	}
}

func TestGetDestinationByPath(t *testing.T) {
	cfg := &Config{
		Destinations: []DestinationConfig{
			{Name: "dest1", Path: "/webhook/dest1", Enabled: true},
			{Name: "dest2", Path: "/webhook/dest2", Enabled: true},
			{Name: "dest3", Path: "/webhook/dest3", Enabled: false},
		},
	}
	
	// Test existing enabled destination
	dest := cfg.GetDestinationByPath("/webhook/dest1")
	require.NotNil(t, dest)
	assert.Equal(t, "dest1", dest.Name)
	
	// Test non-existing destination
	dest = cfg.GetDestinationByPath("/webhook/nonexistent")
	assert.Nil(t, dest)
	
	// Test disabled destination
	dest = cfg.GetDestinationByPath("/webhook/dest3")
	assert.Nil(t, dest)
}

func TestGetDestinationByName(t *testing.T) {
	cfg := &Config{
		Destinations: []DestinationConfig{
			{Name: "dest1", Path: "/webhook/dest1", Enabled: true},
			{Name: "dest2", Path: "/webhook/dest2", Enabled: true},
			{Name: "dest3", Path: "/webhook/dest3", Enabled: false},
		},
	}
	
	// Test existing enabled destination
	dest := cfg.GetDestinationByName("dest1")
	require.NotNil(t, dest)
	assert.Equal(t, "/webhook/dest1", dest.Path)
	
	// Test non-existing destination
	dest = cfg.GetDestinationByName("nonexistent")
	assert.Nil(t, dest)
	
	// Test disabled destination
	dest = cfg.GetDestinationByName("dest3")
	assert.Nil(t, dest)
}

func TestExpandEnvVars(t *testing.T) {
	os.Setenv("TEST_VAR", "test-value")
	os.Setenv("ANOTHER_VAR", "another-value")
	defer func() {
		os.Unsetenv("TEST_VAR")
		os.Unsetenv("ANOTHER_VAR")
	}()
	
	tests := []struct {
		input    string
		expected string
	}{
		{"${TEST_VAR}", "test-value"},
		{"$TEST_VAR", "test-value"},
		{"prefix-${TEST_VAR}-suffix", "prefix-test-value-suffix"},
		{"${TEST_VAR}/${ANOTHER_VAR}", "test-value/another-value"},
		{"${UNSET_VAR:-default}", "default"},
		{"${TEST_VAR:-default}", "test-value"},
		{"no vars here", "no vars here"},
	}
	
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := expandEnvVars(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}