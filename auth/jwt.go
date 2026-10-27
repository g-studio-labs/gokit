// Package auth bundles what the house already uses for authentication, with
// the decisions made: JWT HS256 (jwt/v5) always validating algorithm, issuer
// and expiration; argon2id for passwords (accepting old pbkdf2 and bcrypt
// hashes); prefixed API keys stored as SHA-256; HttpOnly session cookies.
package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/g-studio-labs/gokit/response"
)

// MinSecretLen: HS256 with a secret under 32 bytes is cheap to brute-force.
const MinSecretLen = 32

// Claims is the house's standard payload. A project that needs more defines
// its own embedding jwt.RegisteredClaims and uses the generic Sign/Parse.
type Claims struct {
	jwt.RegisteredClaims
	Role   string   `json:"role,omitempty"`
	Kind   string   `json:"kind,omitempty"` // web | app | service ...
	Scopes []string `json:"scopes,omitempty"`
	Name   string   `json:"name,omitempty"`
	Email  string   `json:"email,omitempty"`
}

// Signer signs and validates tokens for one issuer with one secret.
type Signer struct {
	secret []byte
	issuer string
}

// NewSigner fails if the secret is short. issuer identifies the service
// ("my-api"): a token from another service with the same secret is not valid here.
func NewSigner(secret []byte, issuer string) (*Signer, error) {
	if len(secret) < MinSecretLen {
		return nil, fmt.Errorf("JWT secret must be at least %d bytes (openssl rand -base64 48)", MinSecretLen)
	}
	if issuer == "" {
		return nil, errors.New("JWT issuer cannot be empty")
	}
	return &Signer{secret: secret, issuer: issuer}, nil
}

// Issuer returns the Signer's issuer.
func (s *Signer) Issuer() string { return s.issuer }

// NewClaims builds the registered claims with issuer, subject, iat and exp.
func (s *Signer) NewClaims(subject string, ttl time.Duration) jwt.RegisteredClaims {
	now := time.Now()
	return jwt.RegisteredClaims{
		Issuer:    s.issuer,
		Subject:   subject,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
}

// Sign signs claims with HS256.
func Sign(s *Signer, claims jwt.Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// Parse validates signature, algorithm, issuer and expiration and returns the claims.
// T is the claims type (auth.Claims or a custom one).
func Parse[T any, PT interface {
	*T
	jwt.Claims
}](s *Signer, token string) (*T, error) {
	claims := PT(new(T))
	_, err := jwt.ParseWithClaims(token, claims, func(*jwt.Token) (any, error) { return s.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(s.issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil {
		return nil, err
	}
	return (*T)(claims), nil
}

type claimsKey struct{}

// WithClaims stores the claims in the context (the middlewares do this).
func WithClaims(ctx context.Context, c any) context.Context {
	return context.WithValue(ctx, claimsKey{}, c)
}

// ClaimsFrom retrieves the claims from the context, or nil if there is no session.
func ClaimsFrom[T any](ctx context.Context) *T {
	c, _ := ctx.Value(claimsKey{}).(*T)
	return c
}

// BearerToken extracts the token from "Authorization: Bearer ...", or "".
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// Bearer requires a valid JWT in Authorization and stores the claims in the
// context. Missing or invalid token responds 401 with the envelope.
func Bearer[T any, PT interface {
	*T
	jwt.Claims
}](s *Signer) func(http.Handler) http.Handler {
	return tokenMiddleware[T, PT](s, BearerToken)
}

// Cookie is Bearer but reads the token from a cookie (admin sessions in the
// browser). Combine it with middleware.AntiCSRF.
func Cookie[T any, PT interface {
	*T
	jwt.Claims
}](s *Signer, name string) func(http.Handler) http.Handler {
	return tokenMiddleware[T, PT](s, func(r *http.Request) string {
		c, err := r.Cookie(name)
		if err != nil {
			return ""
		}
		return c.Value
	})
}

func tokenMiddleware[T any, PT interface {
	*T
	jwt.Claims
}](s *Signer, extract func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := extract(r)
			if tok == "" {
				response.Fail(w, r, response.ErrUnauthorized)
				return
			}
			claims, err := Parse[T, PT](s, tok)
			if err != nil {
				response.Fail(w, r, response.Unauthorized("").Wrap(err))
				return
			}
			next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
		})
	}
}

// RequireRole lets the request through only if the session's role is in roles.
// roleOf reads the role from the claims; for auth.Claims use RoleOf.
func RequireRole(roleOf func(ctx context.Context) string, roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			role := roleOf(r.Context())
			if role == "" {
				response.Fail(w, r, response.ErrUnauthorized)
				return
			}
			for _, allowed := range roles {
				if role == allowed {
					next.ServeHTTP(w, r)
					return
				}
			}
			response.Fail(w, r, response.ErrForbidden)
		})
	}
}

// RoleOf is RequireRole's roleOf for auth.Claims.
func RoleOf(ctx context.Context) string {
	if c := ClaimsFrom[Claims](ctx); c != nil {
		return c.Role
	}
	return ""
}
