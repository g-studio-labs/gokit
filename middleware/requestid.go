package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/g-studio-labs/gokit/lg"
)

// RequestIDHeader is the header that is read and returned.
const RequestIDHeader = "X-Request-ID"

// RequestID honors the X-Request-ID sent by the proxy or the client (if it is
// reasonable: up to 64 safe characters) and otherwise generates one. It stores
// it in the context (lg.RequestID) and in the response, to correlate logs with
// what the client saw.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(RequestIDHeader)
		if !validID(id) {
			id = newID()
		}
		w.Header().Set(RequestIDHeader, id)
		next.ServeHTTP(w, r.WithContext(lg.WithRequestID(r.Context(), id)))
	})
}

func newID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func validID(id string) bool {
	if len(id) == 0 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	return true
}
