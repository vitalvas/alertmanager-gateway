package transform

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"github.com/vitalvas/alertmanager-gateway/internal/alertmanager"
	"github.com/vitalvas/alertmanager-gateway/internal/cache"
)

// Global template cache
var (
	globalTemplateCache     *cache.TemplateCache
	globalTemplateCacheOnce sync.Once
)

// InitTemplateCache initializes the global template cache
func InitTemplateCache(maxSize int, ttl time.Duration) {
	globalTemplateCacheOnce.Do(func() {
		globalTemplateCache = cache.NewTemplateCache(maxSize, ttl)
		// Start cleanup task
		globalTemplateCache.StartCleanupTask(5 * time.Minute)
	})
}

// GetTemplateCache returns the global template cache
func GetTemplateCache() *cache.TemplateCache {
	// Initialize with defaults if not already initialized
	InitTemplateCache(1000, 1*time.Hour)
	return globalTemplateCache
}

// CachedGoTemplateEngine implements the Engine interface with caching
type CachedGoTemplateEngine struct {
	templateString string
	templateHash   string
	compiledOnce   sync.Once
	compileError   error
	renderCount    int64
	avgRenderTime  int64 // in nanoseconds
}

// NewCachedGoTemplateEngine creates a new cached Go template engine
func NewCachedGoTemplateEngine(templateString string) (*CachedGoTemplateEngine, error) {
	if templateString == "" {
		return nil, fmt.Errorf("template cannot be empty")
	}

	// Calculate hash of template string
	hasher := md5.New()
	hasher.Write([]byte(templateString))
	hash := hex.EncodeToString(hasher.Sum(nil))

	engine := &CachedGoTemplateEngine{
		templateString: templateString,
		templateHash:   hash,
	}

	// Pre-compile to validate
	if err := engine.ensureCompiled(); err != nil {
		return nil, err
	}

	return engine, nil
}

// ensureCompiled ensures the template is compiled and cached
func (e *CachedGoTemplateEngine) ensureCompiled() error {
	e.compiledOnce.Do(func() {
		cache := GetTemplateCache()

		// Check if template is already in cache
		if _, found := cache.Get(e.templateHash); !found {
			// Compile template
			tmpl := template.New("transform").Funcs(GetTemplateFuncs()).Option("missingkey=default")
			compiled, err := tmpl.Parse(e.templateString)
			if err != nil {
				e.compileError = fmt.Errorf("failed to parse template: %w", err)
				return
			}

			// Store in cache
			cache.Set(e.templateHash, compiled)
		}
	})

	return e.compileError
}

// Transform transforms the webhook payload using the cached template
func (e *CachedGoTemplateEngine) Transform(payload *alertmanager.WebhookPayload) (interface{}, error) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Nanoseconds()
		count := atomic.AddInt64(&e.renderCount, 1)
		// Update average render time using exponential moving average
		oldAvg := atomic.LoadInt64(&e.avgRenderTime)
		newAvg := oldAvg + (duration-oldAvg)/count
		atomic.StoreInt64(&e.avgRenderTime, newAvg)
	}()

	// Ensure template is compiled
	if err := e.ensureCompiled(); err != nil {
		return nil, err
	}

	// Get template from cache
	cache := GetTemplateCache()
	cachedTemplate, found := cache.Get(e.templateHash)
	if !found {
		// This shouldn't happen if ensureCompiled worked correctly
		return nil, fmt.Errorf("template not found in cache")
	}

	tmpl, ok := cachedTemplate.(*template.Template)
	if !ok {
		return nil, fmt.Errorf("invalid cached template type")
	}

	// Execute template with buffer pool
	buf := getBuffer()
	defer putBuffer(buf)

	err := tmpl.Execute(buf, payload)
	if err != nil {
		return nil, fmt.Errorf("template execution failed: %w", err)
	}

	// Try to parse as JSON
	result := buf.String()

	// Fast path: if it looks like JSON, try to parse it
	if len(result) > 0 && (result[0] == '{' || result[0] == '[') {
		var jsonData interface{}
		decoder := json.NewDecoder(bytes.NewReader(buf.Bytes()))
		decoder.UseNumber() // Preserve number precision

		if err := decoder.Decode(&jsonData); err == nil {
			return jsonData, nil
		}
	}

	// Return as string if not JSON
	return result, nil
}

// TransformSplit transforms a single alert using the cached template
func (e *CachedGoTemplateEngine) TransformSplit(payload *alertmanager.WebhookPayload, alert alertmanager.Alert) (interface{}, error) {
	// Create a copy of the payload with only the single alert
	singleAlertPayload := *payload
	singleAlertPayload.Alerts = []alertmanager.Alert{alert}

	// Create split context
	splitContext := struct {
		*alertmanager.WebhookPayload
		Alert alertmanager.Alert
	}{
		WebhookPayload: &singleAlertPayload,
		Alert:          alert,
	}

	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime).Nanoseconds()
		count := atomic.AddInt64(&e.renderCount, 1)
		oldAvg := atomic.LoadInt64(&e.avgRenderTime)
		newAvg := oldAvg + (duration-oldAvg)/count
		atomic.StoreInt64(&e.avgRenderTime, newAvg)
	}()

	// Ensure template is compiled
	if err := e.ensureCompiled(); err != nil {
		return nil, err
	}

	// Get template from cache
	cache := GetTemplateCache()
	cachedTemplate, found := cache.Get(e.templateHash)
	if !found {
		return nil, fmt.Errorf("template not found in cache")
	}

	tmpl, ok := cachedTemplate.(*template.Template)
	if !ok {
		return nil, fmt.Errorf("invalid cached template type")
	}

	// Execute template
	buf := getBuffer()
	defer putBuffer(buf)

	err := tmpl.Execute(buf, splitContext)
	if err != nil {
		return nil, fmt.Errorf("template execution failed: %w", err)
	}

	// Try to parse as JSON
	result := buf.String()
	if len(result) > 0 && (result[0] == '{' || result[0] == '[') {
		var jsonData interface{}
		decoder := json.NewDecoder(bytes.NewReader(buf.Bytes()))
		decoder.UseNumber()

		if err := decoder.Decode(&jsonData); err == nil {
			return jsonData, nil
		}
	}

	return result, nil
}

// Validate validates the template
func (e *CachedGoTemplateEngine) Validate() error {
	return e.ensureCompiled()
}

// Type returns the engine type
func (e *CachedGoTemplateEngine) Type() string {
	return "go-template-cached"
}

// Stats returns rendering statistics
func (e *CachedGoTemplateEngine) Stats() (renderCount int64, avgRenderTimeNs int64) {
	return atomic.LoadInt64(&e.renderCount), atomic.LoadInt64(&e.avgRenderTime)
}

// Buffer pool to reduce allocations
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

func getBuffer() *bytes.Buffer {
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

func putBuffer(buf *bytes.Buffer) {
	// Don't put back buffers that are too large
	if buf.Cap() > 64*1024 {
		return
	}
	bufferPool.Put(buf)
}
