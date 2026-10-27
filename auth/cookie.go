package auth

import (
	"net/http"
	"time"
)

// CookieOptions adapts the session cookie to the deployment.
type CookieOptions struct {
	// Domain: in production the shared domain (".example.com") so the admin
	// sends it to the API; locally, empty.
	Domain string
	// Secure: false only locally without HTTPS.
	Secure bool
	// SameSite: default Lax (the cookie travels on normal navigation, not on a
	// POST from another site). Strict if the admin and the API share a host.
	SameSite http.SameSite
	Path     string
}

// SessionCookie builds an HttpOnly cookie carrying the session (usually a JWT).
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

// ClearCookie is the same cookie, expired, for logout.
func ClearCookie(name string, o CookieOptions) *http.Cookie {
	c := SessionCookie(name, "", 0, o)
	c.MaxAge = -1
	return c
}
