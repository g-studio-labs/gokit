package middleware

import (
	"net/http"
	"slices"

	"github.com/RomanshkVolkov/gokit/response"
)

// CodeOriginNotAllowed es el código del sobre cuando AntiCSRF rechaza.
const CodeOriginNotAllowed = "origin-not-allowed"

// AntiCSRF rechaza peticiones que cambian datos (todo menos GET/HEAD/OPTIONS)
// con un Origin ajeno. Hace falta cuando la sesión va en cookie: el navegador
// la manda sola desde cualquier sitio. Sin Origin se dejan pasar (no vienen
// de un navegador en otro sitio: siempre lo mandan en un POST entre orígenes).
// Los clientes no-navegador (curl, n8n) no mandan Origin y no les afecta.
func AntiCSRF(origins []string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
			default:
				if o := r.Header.Get("Origin"); o != "" && !slices.Contains(origins, o) {
					response.Fail(w, r, response.Forbidden("Origen no permitido").WithCode(CodeOriginNotAllowed))
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
