package httputil

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/trace"
	"golang.org/x/time/rate"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusRecorder) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *statusRecorder) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(body)
	w.bytes += n
	return n, err
}

func Recover(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.ErrorContext(r.Context(), "panic recovered", "request_id", RequestIDFromContext(r.Context()))
					WriteError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred.")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func AccessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			recorder := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(recorder, r)
			attributes := []any{"request_id", RequestIDFromContext(r.Context()), "method", r.Method, "path", r.URL.Path, "status", recorder.status, "bytes", recorder.bytes, "duration_ms", time.Since(started).Milliseconds()}
			if span := trace.SpanContextFromContext(r.Context()); span.IsValid() {
				attributes = append(attributes, "trace_id", span.TraceID().String())
			}
			logger.InfoContext(r.Context(), "http request", attributes...)
		})
	}
}

func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

func BodyLimit(bytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, bytes)
			next.ServeHTTP(w, r)
		})
	}
}

func RequireJSON(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil && r.ContentLength != 0 && !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
			WriteError(w, r, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type must be application/json.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func RateLimit(requestsPerSecond float64, burst int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(rate.Limit(requestsPerSecond), burst)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				WriteError(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "Too many requests.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
