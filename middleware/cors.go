package middleware

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// CORSOptions configura CORS. Origins es una lista blanca exacta
// ("https://admin.example.com"); "*" solo se acepta sin credenciales, porque
// con cookies el navegador lo rechaza y además abriría la sesión a cualquiera.
type CORSOptions struct {
	Origins          []string
	AllowCredentials bool
	Methods          []string // default GET, POST, PUT, PATCH, DELETE
	Headers          []string // default Content-Type, Authorization, X-Request-ID
	ExposeHeaders    []string // default X-Request-ID, X-Response-Time
	MaxAge           int      // segundos de caché del preflight; default 600
}

// CORS responde el preflight y pone los headers solo para orígenes de la lista.
// Sin Origins no hace nada (front same-origin): mejor que un "*" por defecto.
func CORS(o CORSOptions) Middleware {
	if len(o.Methods) == 0 {
		o.Methods = []string{"GET", "POST", "PUT", "PATCH", "DELETE"}
	}
	if len(o.Headers) == 0 {
		o.Headers = []string{"Content-Type", "Authorization", RequestIDHeader}
	}
	if len(o.ExposeHeaders) == 0 {
		o.ExposeHeaders = []string{RequestIDHeader, "X-Response-Time"}
	}
	if o.MaxAge == 0 {
		o.MaxAge = 600
	}
	wildcard := slices.Contains(o.Origins, "*") && !o.AllowCredentials
	methods := strings.Join(o.Methods, ", ")
	headers := strings.Join(o.Headers, ", ")
	expose := strings.Join(o.ExposeHeaders, ", ")
	maxAge := strconv.Itoa(o.MaxAge)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			allowed := origin != "" && (wildcard || slices.Contains(o.Origins, origin))
			if allowed {
				h := w.Header()
				if wildcard {
					h.Set("Access-Control-Allow-Origin", "*")
				} else {
					h.Set("Access-Control-Allow-Origin", origin)
					h.Add("Vary", "Origin")
				}
				if o.AllowCredentials {
					h.Set("Access-Control-Allow-Credentials", "true")
				}
				h.Set("Access-Control-Expose-Headers", expose)
				if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
					h.Set("Access-Control-Allow-Methods", methods)
					h.Set("Access-Control-Allow-Headers", headers)
					h.Set("Access-Control-Max-Age", maxAge)
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
