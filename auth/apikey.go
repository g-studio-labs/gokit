package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/g-studio-labs/gokit/response"
)

// NewAPIKey genera una llave "<prefix>_<48 hex>" y su hash. La llave en claro
// se muestra una sola vez; en la base queda el hash. SHA-256 alcanza (y
// permite buscar por hash) porque la llave tiene 192 bits de azar; argon2 es
// para contraseñas elegidas por personas.
func NewAPIKey(prefix string) (plain, hash string, err error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain = prefix + "_" + hex.EncodeToString(b)
	return plain, HashAPIKey(plain), nil
}

// HashAPIKey devuelve el SHA-256 en hex de la llave.
func HashAPIKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// APIKeyOptions configura RequireAPIKey.
type APIKeyOptions struct {
	// Header donde viene la llave; default X-API-Key.
	Header string
	// Lookup recibe el hash de la llave y devuelve el valor que se dejará en
	// el contexto (cuenta, cliente...) y si es válida. Buscar por hash en la
	// base es un índice normal.
	Lookup func(ctx context.Context, hash string) (any, bool)
}

type apiKeyKey struct{}

// RequireAPIKey exige una llave válida y deja lo que devolvió Lookup en el
// contexto (APIKeyPrincipal). Sin llave o inválida responde 401.
func RequireAPIKey(o APIKeyOptions) func(http.Handler) http.Handler {
	if o.Header == "" {
		o.Header = "X-API-Key"
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get(o.Header)
			if key == "" {
				response.Fail(w, r, response.ErrUnauthorized)
				return
			}
			principal, ok := o.Lookup(r.Context(), HashAPIKey(key))
			if !ok {
				response.Fail(w, r, response.ErrUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), apiKeyKey{}, principal)))
		})
	}
}

// APIKeyPrincipal devuelve lo que Lookup dejó en el contexto, o nil.
func APIKeyPrincipal[T any](ctx context.Context) *T {
	p, _ := ctx.Value(apiKeyKey{}).(*T)
	return p
}
