package middleware

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/RomanshkVolkov/gokit/lg"
)

// LoggerOptions ajusta el log de peticiones.
type LoggerOptions struct {
	// Skip decide qué peticiones no se registran. Por defecto, /health, /live
	// y /ready (los healthchecks no aportan).
	Skip func(r *http.Request) bool
	// RedactParams son parámetros de query que se tapan en el log. Se suman a
	// los de siempre: token, key, pass, password, secret, access_token, api_key.
	RedactParams []string
}

var defaultRedact = []string{"token", "key", "pass", "password", "secret", "access_token", "api_key", "apikey", "hub.verify_token"}

// Logger registra una línea por petición: método, URI (con la query sensible
// tapada), status, duración e IP. 2xx/3xx a Info, 4xx a Warn, 5xx a Error.
// Añade X-Response-Time y deja pasar Flush (SSE) y Unwrap
// (http.ResponseController) al writer real.
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

// statusWriter recuerda el status para el log y pone X-Response-Time justo
// antes de mandar los headers, que es el último momento en que se puede.
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

// Unwrap deja que http.ResponseController llegue al writer real.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// Flush deja pasar el flush del SSE.
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
