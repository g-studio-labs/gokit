package middleware

import (
	"net/http"
	"slices"

	"github.com/g-studio-labs/gokit/response"
)

// CodeOriginNotAllowed is the envelope code when AntiCSRF rejects.
const CodeOriginNotAllowed = "origin-not-allowed"

// AntiCSRF rejects data-changing requests (everything but GET/HEAD/OPTIONS)
// with a foreign Origin. It is needed when the session travels in a cookie: the
// browser sends it on its own from any site. Requests without Origin pass (they
// do not come from a browser on another site: browsers always send it on a
// cross-origin POST). Non-browser clients (curl, n8n) send no Origin and are unaffected.
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
