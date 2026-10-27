package middleware

import "net/http"

// HeaderOptions tunes SecurityHeaders.
type HeaderOptions struct {
	// HSTS enables Strict-Transport-Security. Only if the service is always
	// served over HTTPS (behind Caddy it is). Caddy already sets it if asked;
	// repeating it does no harm.
	HSTS bool
	// CSP is the Content-Security-Policy. For a JSON API the default is enough
	// ("default-src 'none'; frame-ancestors 'none'"), which keeps a response
	// rendered by mistake from executing anything.
	CSP string
}

// SecurityHeaders sets the headers a JSON API must always carry. It goes at
// the start of the chain so it also covers errors and 404s.
func SecurityHeaders(o HeaderOptions) Middleware {
	if o.CSP == "" {
		o.CSP = "default-src 'none'; frame-ancestors 'none'"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			h.Set("Content-Security-Policy", o.CSP)
			h.Set("Cache-Control", "no-store")
			if o.HSTS {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}
