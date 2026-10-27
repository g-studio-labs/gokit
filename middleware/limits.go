package middleware

import (
	"context"
	"net/http"
	"time"
)

// MaxBody limits every request body to n bytes. response.Decode already
// limits JSON; this covers the rest (multipart, raw) and avoids overreading.
// When exceeded, the next Read returns *http.MaxBytesError and response.Fail
// answers it as 413.
func MaxBody(n int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, n)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Timeout puts a deadline on the request context. It does not stop the handler
// (that is not possible in Go): whatever honors the context (DB, outgoing HTTP)
// fails with context.DeadlineExceeded and response.Fail answers it as 504. For
// SSE or streaming, do not apply it to those routes.
func Timeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
