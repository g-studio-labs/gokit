package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"

	"github.com/g-studio-labs/gokit/response"
)

// NewAPIKey generates a "<prefix>_<48 hex>" key and its hash. The plaintext
// key is shown only once; the database keeps the hash. SHA-256 is enough (and
// allows lookup by hash) because the key has 192 bits of randomness; argon2 is
// for passwords chosen by people.
func NewAPIKey(prefix string) (plain, hash string, err error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain = prefix + "_" + hex.EncodeToString(b)
	return plain, HashAPIKey(plain), nil
}

// HashAPIKey returns the hex SHA-256 of the key.
func HashAPIKey(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// APIKeyOptions configures RequireAPIKey.
type APIKeyOptions struct {
	// Header carrying the key; default X-API-Key.
	Header string
	// Lookup receives the key's hash and returns the value to store in the
	// context (account, client...) and whether it is valid. Looking up by hash
	// in the database is a regular index.
	Lookup func(ctx context.Context, hash string) (any, bool)
}

type apiKeyKey struct{}

// RequireAPIKey requires a valid key and stores what Lookup returned in the
// context (APIKeyPrincipal). Missing or invalid key responds 401.
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

// APIKeyPrincipal returns what Lookup stored in the context, or nil.
func APIKeyPrincipal[T any](ctx context.Context) *T {
	p, _ := ctx.Value(apiKeyKey{}).(*T)
	return p
}
