package auth

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/vitalvas/alertmanager-gateway/internal/config"
)

// Authenticator handles authentication logic
type Authenticator struct {
	config      *config.AuthConfig
	logger      *logrus.Entry
	rateLimiter *RateLimiter
}

// NewAuthenticator creates a new authenticator instance
func NewAuthenticator(cfg *config.AuthConfig, logger *logrus.Logger) *Authenticator {
	rateLimiter := NewRateLimiter(logger)
	rateLimiter.StartCleanupTimer()

	return &Authenticator{
		config:      cfg,
		logger:      logger.WithField("component", "auth"),
		rateLimiter: rateLimiter,
	}
}

// ValidateCredentials validates username and password using constant-time comparison
func (a *Authenticator) ValidateCredentials(username, password string) bool {
	if !a.config.Enabled {
		return true // Authentication disabled
	}

	if a.config.Username == "" || a.config.Password == "" {
		a.logger.Warn("Authentication enabled but credentials not configured")
		return false
	}

	// Use constant-time comparison to prevent timing attacks
	validUsername := subtle.ConstantTimeCompare([]byte(username), []byte(a.config.Username)) == 1
	validPassword := subtle.ConstantTimeCompare([]byte(password), []byte(a.config.Password)) == 1

	return validUsername && validPassword
}

// ValidateAPICredentials validates API credentials with fallback to regular credentials
func (a *Authenticator) ValidateAPICredentials(username, password string) bool {
	if !a.config.Enabled {
		return true // Authentication disabled
	}

	// Check API credentials first if configured
	if a.config.APIUsername != "" && a.config.APIPassword != "" {
		validAPIUser := subtle.ConstantTimeCompare([]byte(username), []byte(a.config.APIUsername)) == 1
		validAPIPass := subtle.ConstantTimeCompare([]byte(password), []byte(a.config.APIPassword)) == 1
		if validAPIUser && validAPIPass {
			return true
		}
	}

	// Fallback to regular credentials
	return a.ValidateCredentials(username, password)
}

// BasicAuthMiddleware creates a middleware for HTTP Basic Authentication
func (a *Authenticator) BasicAuthMiddleware(next http.Handler) http.Handler {
	return a.createAuthMiddleware(next, a.ValidateCredentials)
}

// APIAuthMiddleware creates a middleware for API authentication with dual credential support
func (a *Authenticator) APIAuthMiddleware(next http.Handler) http.Handler {
	return a.createAuthMiddleware(next, a.ValidateAPICredentials)
}

// createAuthMiddleware creates a middleware with the given validation function
func (a *Authenticator) createAuthMiddleware(next http.Handler, validateFunc func(string, string) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.config.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Check rate limiting
		if !a.rateLimiter.IsAllowed(r) {
			a.sendRateLimited(w, r)
			return
		}

		username, password, ok := r.BasicAuth()
		if !ok {
			a.sendUnauthorized(w, r, "Basic authentication required")
			return
		}

		if !validateFunc(username, password) {
			a.logFailedAuth(r, username, "invalid credentials")
			a.rateLimiter.RecordFailedAttempt(r)
			a.sendUnauthorized(w, r, "Invalid credentials")
			return
		}

		a.logSuccessfulAuth(r, username)
		a.rateLimiter.RecordSuccessfulAttempt(r)
		next.ServeHTTP(w, r)
	})
}

// sendUnauthorized sends a standardized 401 response
func (a *Authenticator) sendUnauthorized(w http.ResponseWriter, _ *http.Request, message string) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Alertmanager Gateway"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	fmt.Fprintf(w, `{"status":"error","error":"%s","timestamp":"%s"}`,
		message, time.Now().UTC().Format(time.RFC3339))
}

// sendRateLimited sends a standardized 429 response
func (a *Authenticator) sendRateLimited(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "900") // 15 minutes
	w.WriteHeader(http.StatusTooManyRequests)
	fmt.Fprintf(w, `{"status":"error","error":"Too many authentication attempts. Please try again later.","timestamp":"%s"}`,
		time.Now().UTC().Format(time.RFC3339))
}

// logSuccessfulAuth logs successful authentication attempts
func (a *Authenticator) logSuccessfulAuth(r *http.Request, username string) {
	a.logger.WithFields(logrus.Fields{
		"username":   username,
		"remote_ip":  getClientIP(r),
		"user_agent": r.UserAgent(),
		"method":     r.Method,
		"path":       r.URL.Path,
	}).Info("Authentication successful")
}

// logFailedAuth logs failed authentication attempts
func (a *Authenticator) logFailedAuth(r *http.Request, username, reason string) {
	a.logger.WithFields(logrus.Fields{
		"username":   username,
		"remote_ip":  getClientIP(r),
		"user_agent": r.UserAgent(),
		"method":     r.Method,
		"path":       r.URL.Path,
		"reason":     reason,
	}).Warn("Authentication failed")
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}

	// Check X-Real-IP header (for proxies)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// IsEnabled returns whether authentication is enabled
func (a *Authenticator) IsEnabled() bool {
	return a.config.Enabled
}

// HasAPICredentials returns whether separate API credentials are configured
func (a *Authenticator) HasAPICredentials() bool {
	return a.config.APIUsername != "" && a.config.APIPassword != ""
}

// GetRateLimitStats returns rate limiting statistics
func (a *Authenticator) GetRateLimitStats() map[string]interface{} {
	return a.rateLimiter.GetStats()
}
