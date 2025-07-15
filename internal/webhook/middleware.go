package webhook

import (
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"
)

// ValidationMiddleware provides request validation for webhook endpoints
func ValidationMiddleware(logger *logrus.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only validate POST requests
			if r.Method != http.MethodPost {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				return
			}

			// Check Content-Type
			contentType := r.Header.Get("Content-Type")
			if contentType != "" && !strings.HasPrefix(contentType, "application/json") {
				logger.WithField("content_type", contentType).Warn("Non-JSON content type received")
			}

			// Validate required headers
			if r.ContentLength == 0 {
				http.Error(w, "Empty request body", http.StatusBadRequest)
				return
			}

			// Check for Alertmanager-specific headers (optional but good to log)
			if userAgent := r.Header.Get("User-Agent"); userAgent != "" {
				if strings.Contains(userAgent, "Alertmanager") {
					logger.WithField("user_agent", userAgent).Debug("Request from Alertmanager")
				}
			}

			next.ServeHTTP(w, r)
		})
	}
}
