package middleware

import (
	"context"
	"net/http"
	"time"
)

// MaxBody limita el cuerpo de toda petición a n bytes. response.Decode ya
// limita los JSON; esto cubre lo demás (multipart, raw) y evita leer de más.
// Cuando se pasa, el siguiente Read devuelve *http.MaxBytesError y
// response.Fail lo responde como 413.
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

// Timeout pone un plazo al contexto de la petición. No corta el handler (eso
// no se puede en Go): lo que respeta el contexto (DB, HTTP salientes) falla
// con context.DeadlineExceeded y response.Fail lo responde como 504. Para
// SSE o streaming, no lo apliques a esas rutas.
func Timeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), d)
			defer cancel()
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
