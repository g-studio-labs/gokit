package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/RomanshkVolkov/gokit/lg"
)

// RequestIDHeader es el header que se lee y se devuelve.
const RequestIDHeader = "X-Request-ID"

// RequestID respeta el X-Request-ID que mande el proxy o el cliente (si es
// razonable: hasta 64 caracteres seguros) y si no genera uno. Lo deja en el
// contexto (lg.RequestID) y en la respuesta, para correlacionar logs con lo
// que vio el cliente.
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
