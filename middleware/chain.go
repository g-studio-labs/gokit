// Package middleware bundles the house's net/http middlewares, designed for
// plain net/http (they also work in chi). The recommended order, from
// outermost to innermost:
//
//	h := middleware.Chain(mux,
//	    middleware.Recover,
//	    middleware.RequestID,
//	    middleware.RealIP(),            // only if there is a proxy in front (Caddy, Envoy, CF)
//	    middleware.Logger(middleware.LoggerOptions{}),
//	    middleware.SecurityHeaders(middleware.HeaderOptions{}),
//	    middleware.CORS(middleware.CORSOptions{Origins: origins, AllowCredentials: true}),
//	    middleware.AntiCSRF(origins),   // if the session travels in a cookie
//	    middleware.MaxBody(1<<20),
//	)
package middleware

import (
	"net/http"
	"slices"
)

// Middleware wraps a handler.
type Middleware = func(http.Handler) http.Handler

// Chain applies the middlewares in order: the first is the outermost.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for _, mw := range slices.Backward(mws) {
		h = mw(h)
	}
	return h
}
