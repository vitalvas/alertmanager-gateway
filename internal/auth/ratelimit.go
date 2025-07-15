package auth

import (
	"net/http"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// RateLimiter provides rate limiting for authentication attempts
type RateLimiter struct {
	attempts map[string]*attemptRecord
	mu       sync.RWMutex
	logger   *logrus.Entry

	// Configuration
	maxAttempts int
	windowSize  time.Duration
	banDuration time.Duration
}

// attemptRecord tracks authentication attempts for an IP
type attemptRecord struct {
	count        int
	firstAttempt time.Time
	bannedUntil  time.Time
}

// NewRateLimiter creates a new rate limiter for authentication
func NewRateLimiter(logger *logrus.Logger) *RateLimiter {
	return &RateLimiter{
		attempts:    make(map[string]*attemptRecord),
		logger:      logger.WithField("component", "auth-ratelimit"),
		maxAttempts: 5,                // Max 5 attempts
		windowSize:  time.Minute,      // In 1 minute window
		banDuration: 15 * time.Minute, // Ban for 15 minutes
	}
}

// SetBanDuration sets the ban duration (for testing)
func (rl *RateLimiter) SetBanDuration(duration time.Duration) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.banDuration = duration
}

// IsAllowed checks if an IP is allowed to attempt authentication
func (rl *RateLimiter) IsAllowed(r *http.Request) bool {
	ip := getClientIP(r)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	record, exists := rl.attempts[ip]

	if !exists {
		// First attempt from this IP
		rl.attempts[ip] = &attemptRecord{
			count:        0,
			firstAttempt: now,
		}
		return true
	}

	// Check if currently banned
	if now.Before(record.bannedUntil) {
		rl.logger.WithFields(logrus.Fields{
			"ip":           ip,
			"banned_until": record.bannedUntil,
		}).Warn("Authentication attempt from banned IP")
		return false
	}

	// If ban has expired but count is still at max, reset the counter
	if !record.bannedUntil.IsZero() && now.After(record.bannedUntil) && record.count >= rl.maxAttempts {
		record.count = 0
		record.firstAttempt = now
		record.bannedUntil = time.Time{}
	}

	// Reset counter if window has expired
	if now.Sub(record.firstAttempt) > rl.windowSize {
		record.count = 0
		record.firstAttempt = now
		record.bannedUntil = time.Time{}
	}

	return record.count < rl.maxAttempts
}

// RecordFailedAttempt records a failed authentication attempt
func (rl *RateLimiter) RecordFailedAttempt(r *http.Request) {
	ip := getClientIP(r)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	record, exists := rl.attempts[ip]

	if !exists {
		record = &attemptRecord{
			count:        0,
			firstAttempt: now,
		}
		rl.attempts[ip] = record
	}

	record.count++

	if record.count >= rl.maxAttempts {
		record.bannedUntil = now.Add(rl.banDuration)
		rl.logger.WithFields(logrus.Fields{
			"ip":           ip,
			"attempts":     record.count,
			"banned_until": record.bannedUntil,
		}).Warn("IP banned due to excessive failed authentication attempts")
	}
}

// RecordSuccessfulAttempt resets the failed attempt counter for an IP
func (rl *RateLimiter) RecordSuccessfulAttempt(r *http.Request) {
	ip := getClientIP(r)

	rl.mu.Lock()
	defer rl.mu.Unlock()

	if record, exists := rl.attempts[ip]; exists {
		record.count = 0
		record.bannedUntil = time.Time{}
	}
}

// CleanupExpiredRecords removes old records to prevent memory leaks
func (rl *RateLimiter) CleanupExpiredRecords() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	expiredIPs := make([]string, 0)

	for ip, record := range rl.attempts {
		// Remove records that are older than the ban duration and not currently banned
		if now.Sub(record.firstAttempt) > rl.banDuration && now.After(record.bannedUntil) {
			expiredIPs = append(expiredIPs, ip)
		}
	}

	for _, ip := range expiredIPs {
		delete(rl.attempts, ip)
	}

	if len(expiredIPs) > 0 {
		rl.logger.WithField("cleaned_records", len(expiredIPs)).Debug("Cleaned up expired rate limit records")
	}
}

// StartCleanupTimer starts a timer to periodically clean up expired records
func (rl *RateLimiter) StartCleanupTimer() {
	ticker := time.NewTicker(5 * time.Minute) // Cleanup every 5 minutes
	go func() {
		for range ticker.C {
			rl.CleanupExpiredRecords()
		}
	}()
}

// GetStats returns current rate limiting statistics
func (rl *RateLimiter) GetStats() map[string]interface{} {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	now := time.Now()
	totalIPs := len(rl.attempts)
	bannedIPs := 0

	for _, record := range rl.attempts {
		if now.Before(record.bannedUntil) {
			bannedIPs++
		}
	}

	return map[string]interface{}{
		"total_tracked_ips": totalIPs,
		"currently_banned":  bannedIPs,
		"max_attempts":      rl.maxAttempts,
		"window_size":       rl.windowSize.String(),
		"ban_duration":      rl.banDuration.String(),
	}
}

// RateLimitMiddleware creates a middleware that enforces rate limiting
func (rl *RateLimiter) RateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.IsAllowed(r) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "900") // 15 minutes
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"status":"error","error":"Too many authentication attempts. Please try again later."}`))
			return
		}

		next.ServeHTTP(w, r)
	})
}
