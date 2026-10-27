package middleware

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// CORSOptions configures CORS. Origins is an exact allowlist
// ("https://admin.example.com"); "*" is only accepted without credentials,
// because with cookies the browser rejects it and it would also open the session to anyone.
type CORSOptions struct {
	Origins          []string
	AllowCredentials bool
	Methods          []string // default GET, POST, PUT, PATCH, DELETE
	Headers          []string // default Content-Type, Authorization, X-Request-ID
	ExposeHeaders    []string // default X-Request-ID, X-Response-Time
	MaxAge           int      // preflight cache seconds; default 600
}

// CORS answers the preflight and sets the headers only for listed origins.
// Without Origins it does nothing (same-origin front): better than a default "*".
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
