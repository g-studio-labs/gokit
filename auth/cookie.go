package auth

import (
	"net/http"
	"time"
)

// CookieOptions ajusta la cookie de sesión al despliegue.
type CookieOptions struct {
	// Domain: en producción el dominio compartido (".example.com") para que el
	// admin la mande al API; en local, vacío.
	Domain string
	// Secure: false solo en local sin HTTPS.
	Secure bool
	// SameSite: default Lax (la cookie viaja en navegación normal, no en POST
	// desde otro sitio). Strict si el admin y el API están en el mismo host.
	SameSite http.SameSite
	Path     string
}

// SessionCookie arma una cookie HttpOnly con la sesión (un JWT, normalmente).
func SessionCookie(name, value string, ttl time.Duration, o CookieOptions) *http.Cookie {
	if o.SameSite == 0 {
		o.SameSite = http.SameSiteLaxMode
	}
	if o.Path == "" {
		o.Path = "/"
	}
	return &http.Cookie{
		Name: name, Value: value, Path: o.Path, Domain: o.Domain,
		MaxAge: int(ttl.Seconds()), HttpOnly: true, Secure: o.Secure, SameSite: o.SameSite,
	}
}

// ClearCookie es la misma cookie vencida, para el logout.
func ClearCookie(name string, o CookieOptions) *http.Cookie {
	c := SessionCookie(name, "", 0, o)
	c.MaxAge = -1
	return c
}
