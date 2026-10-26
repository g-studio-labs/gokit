// Package auth trae lo que la casa ya usa para autenticar, con las decisiones
// tomadas: JWT HS256 (jwt/v5) siempre validando algoritmo, emisor y
// expiración; argon2id para contraseñas (aceptando los hashes viejos de
// pbkdf2 y bcrypt); API keys con prefijo guardadas como SHA-256; cookies de
// sesión HttpOnly.
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

// MinSecretLen: HS256 con menos de 32 bytes de secreto es fuerza bruta barata.
const MinSecretLen = 32

// Claims es el payload estándar de la casa. Un proyecto que necesite más
// define el suyo embebiendo jwt.RegisteredClaims y usa Sign/Parse genéricos.
type Claims struct {
	jwt.RegisteredClaims
	Role   string   `json:"role,omitempty"`
	Kind   string   `json:"kind,omitempty"` // web | app | service ...
	Scopes []string `json:"scopes,omitempty"`
	Name   string   `json:"name,omitempty"`
	Email  string   `json:"email,omitempty"`
}

// Signer firma y valida tokens de un emisor con un secreto.
type Signer struct {
	secret []byte
	issuer string
}

// NewSigner falla si el secreto es corto. issuer identifica al servicio
// ("eleva-api"): un token de otro servicio con el mismo secreto no vale aquí.
func NewSigner(secret []byte, issuer string) (*Signer, error) {
	if len(secret) < MinSecretLen {
		return nil, fmt.Errorf("el secreto JWT debe tener al menos %d bytes (openssl rand -base64 48)", MinSecretLen)
	}
	if issuer == "" {
		return nil, errors.New("el emisor JWT no puede ir vacío")
	}
	return &Signer{secret: secret, issuer: issuer}, nil
}

// Issuer devuelve el emisor del Signer.
func (s *Signer) Issuer() string { return s.issuer }

// NewClaims arma los claims registrados con emisor, sujeto, iat y exp.
func (s *Signer) NewClaims(subject string, ttl time.Duration) jwt.RegisteredClaims {
	now := time.Now()
	return jwt.RegisteredClaims{
		Issuer:    s.issuer,
		Subject:   subject,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
}

// Sign firma claims con HS256.
func Sign(s *Signer, claims jwt.Claims) (string, error) {
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

// Parse valida firma, algoritmo, emisor y expiración y devuelve los claims.
// T es el tipo de claims (auth.Claims o uno propio).
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

// WithClaims guarda los claims en el contexto (lo hacen los middlewares).
func WithClaims(ctx context.Context, c any) context.Context {
	return context.WithValue(ctx, claimsKey{}, c)
}

// ClaimsFrom recupera los claims del contexto, o nil si no hay sesión.
func ClaimsFrom[T any](ctx context.Context) *T {
	c, _ := ctx.Value(claimsKey{}).(*T)
	return c
}

// BearerToken saca el token de "Authorization: Bearer ...", o "".
func BearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

// Bearer exige un JWT válido en Authorization y deja los claims en el
// contexto. Sin token o inválido responde 401 con el sobre.
func Bearer[T any, PT interface {
	*T
	jwt.Claims
}](s *Signer) func(http.Handler) http.Handler {
	return tokenMiddleware[T, PT](s, BearerToken)
}

// Cookie es Bearer pero leyendo el token de una cookie (sesiones de admin
// en el navegador). Combínalo con middleware.AntiCSRF.
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

// RequireRole deja pasar solo si el rol de la sesión está en roles. roleOf
// lee el rol de los claims; para auth.Claims usa RoleOf.
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

// RoleOf es el roleOf de RequireRole para auth.Claims.
func RoleOf(ctx context.Context) string {
	if c := ClaimsFrom[Claims](ctx); c != nil {
		return c.Role
	}
	return ""
}
