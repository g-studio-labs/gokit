package middleware

import (
	"fmt"
	"net/http"
	"runtime/debug"

	"github.com/RomanshkVolkov/gokit/lg"
	"github.com/RomanshkVolkov/gokit/response"
)

// Recover atrapa panics, los registra con stack y responde 500 con el sobre.
// http.ErrAbortHandler se vuelve a lanzar: así aborta el proxy inverso cuando
// el cliente corta, y no es un error nuestro.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				if rec == http.ErrAbortHandler {
					panic(rec)
				}
				lg.Ctx(r.Context()).Error("panic", "method", r.Method, "path", r.URL.Path, "panic", fmt.Sprint(rec), "stack", string(debug.Stack()))
				response.Fail(w, r, response.Internal(nil))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
