package middleware

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/g-studio-labs/gokit/lg"
)

// LoggerOptions tunes the request log.
type LoggerOptions struct {
	// Skip decides which requests are not logged. By default, /health, /live
	// and /ready (healthchecks add nothing).
	Skip func(r *http.Request) bool
	// RedactParams are query parameters masked in the log. They add to the
	// usual ones: token, key, pass, password, secret, access_token, api_key.
	RedactParams []string
}

var defaultRedact = []string{"token", "key", "pass", "password", "secret", "access_token", "api_key", "apikey", "hub.verify_token"}

// Logger logs one line per request: method, URI (with the sensitive query
// masked), status, duration and IP. 2xx/3xx to Info, 4xx to Warn, 5xx to Error.
// It adds X-Response-Time and passes Flush (SSE) and Unwrap
// (http.ResponseController) through to the real writer.
func Logger(o LoggerOptions) Middleware {
	skip := o.Skip
	if skip == nil {
		skip = func(r *http.Request) bool {
			switch r.URL.Path {
			case "/health", "/live", "/ready", "/healthz", "/api/health", "/api/live":
				return true
			}
			return false
		}
	}
	redact := append(append([]string(nil), defaultRedact...), o.RedactParams...)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, start: start}
			next.ServeHTTP(sw, r)
			if skip(r) {
				return
			}
			dur := time.Since(start)
			status := sw.status
			if status == 0 {
				status = http.StatusOK
			}
			msg := fmt.Sprintf("%s %s %d %s", r.Method, redactQuery(r.RequestURI, redact), status, dur.Round(time.Millisecond))
			attrs := []any{"method", r.Method, "path", r.URL.Path, "status", status, "ms", dur.Milliseconds(), "ip", ClientIP(r)}
			l := lg.Ctx(r.Context())
			switch {
			case status >= 500:
				l.Error(msg, attrs...)
			case status >= 400:
				l.Warn(msg, attrs...)
			default:
				l.Info(msg, attrs...)
			}
		})
	}
}

// statusWriter remembers the status for the log and sets X-Response-Time right
// before sending the headers, which is the last moment it can be done.
type statusWriter struct {
	http.ResponseWriter
	status int
	start  time.Time
}

func (w *statusWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
		w.Header().Set("X-Response-Time", fmt.Sprintf("%dms", time.Since(w.start).Milliseconds()))
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the real writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Flush passes the SSE flush through.
func (w *statusWriter) Flush() {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func redactQuery(uri string, params []string) string {
	i := strings.IndexByte(uri, '?')
	if i < 0 {
		return uri
	}
	path, raw := uri[:i], uri[i+1:]
	values, err := url.ParseQuery(raw)
	if err != nil {
		return path + "?<unparseable>"
	}
	for _, k := range params {
		if values.Has(k) {
			values.Set(k, "REDACTED")
		}
	}
	return path + "?" + values.Encode()
}
