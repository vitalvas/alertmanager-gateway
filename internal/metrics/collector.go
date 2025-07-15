package metrics

import (
	"runtime"
	"time"

	"github.com/sirupsen/logrus"
)

// SystemCollector collects system-level metrics
type SystemCollector struct {
	metrics *Metrics
	logger  *logrus.Entry
	ticker  *time.Ticker
	done    chan bool
}

// NewSystemCollector creates a new system metrics collector
func NewSystemCollector(metrics *Metrics, logger *logrus.Logger) *SystemCollector {
	return &SystemCollector{
		metrics: metrics,
		logger:  logger.WithField("component", "metrics-collector"),
		ticker:  time.NewTicker(30 * time.Second), // Collect every 30 seconds
		done:    make(chan bool),
	}
}

// Start begins collecting system metrics
func (c *SystemCollector) Start() {
	c.logger.Info("Starting system metrics collection")

	go func() {
		for {
			select {
			case <-c.ticker.C:
				c.collectSystemMetrics()
			case <-c.done:
				c.logger.Info("Stopping system metrics collection")
				return
			}
		}
	}()
}

// Stop stops the metrics collector
func (c *SystemCollector) Stop() {
	c.logger.Info("Stopping system metrics collector")
	c.ticker.Stop()
	close(c.done)
}

// collectSystemMetrics gathers various system metrics
func (c *SystemCollector) collectSystemMetrics() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Update memory usage (allocated bytes)
	c.metrics.MemoryUsage.Set(float64(m.Alloc))

	c.logger.WithFields(logrus.Fields{
		"alloc_bytes": m.Alloc,
		"sys_bytes":   m.Sys,
		"num_gc":      m.NumGC,
		"goroutines":  runtime.NumGoroutine(),
	}).Debug("Collected system metrics")
}

// RecordConfigReload records a configuration reload event
func (c *SystemCollector) RecordConfigReload(success bool) {
	status := "failure"
	if success {
		status = "success"
	}

	c.metrics.ConfigReloads.WithLabelValues(status).Inc()

	c.logger.WithFields(logrus.Fields{
		"status": status,
	}).Info("Recorded config reload")
}

// UpdateBannedIPs updates the banned IPs gauge
func (c *SystemCollector) UpdateBannedIPs(count int) {
	c.metrics.BannedIPs.Set(float64(count))
}

// GetMetrics returns the metrics instance
func (c *SystemCollector) GetMetrics() *Metrics {
	return c.metrics
}
