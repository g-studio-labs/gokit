// Package middleware trae los middlewares net/http de la casa, pensados para
// net/http puro (también sirven en chi). El orden recomendado, del más externo
// al más interno:
//
//	h := middleware.Chain(mux,
//	    middleware.Recover,
//	    middleware.RequestID,
//	    middleware.RealIP(),            // solo si hay proxy delante (Caddy, Envoy, CF)
//	    middleware.Logger(middleware.LoggerOptions{}),
//	    middleware.SecurityHeaders(middleware.HeaderOptions{}),
//	    middleware.CORS(middleware.CORSOptions{Origins: origins, AllowCredentials: true}),
//	    middleware.AntiCSRF(origins),   // si la sesión va en cookie
//	    middleware.MaxBody(1<<20),
//	)
package middleware

import (
	"net/http"
	"slices"
)

// Middleware envuelve un handler.
type Middleware = func(http.Handler) http.Handler

// Chain aplica los middlewares en orden: el primero es el más externo.
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for _, mw := range slices.Backward(mws) {
		h = mw(h)
	}
	return h
}
