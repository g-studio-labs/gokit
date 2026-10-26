package middleware

import "net/http"

// HeaderOptions ajusta SecurityHeaders.
type HeaderOptions struct {
	// HSTS activa Strict-Transport-Security. Solo si el servicio se sirve
	// siempre por HTTPS (detrás de Caddy sí). Caddy ya lo pone si se le pide;
	// repetirlo no estorba.
	HSTS bool
	// CSP es el Content-Security-Policy. Para un API JSON basta el default
	// ("default-src 'none'; frame-ancestors 'none'"), que evita que una
	// respuesta renderizada por error ejecute algo.
	CSP string
}

// SecurityHeaders pone los headers que un API JSON debe llevar siempre. Va al
// principio de la cadena para que también cubra errores y 404.
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
