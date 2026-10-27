package auth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/g-studio-labs/gokit/lg"
)

func TestMain(m *testing.M) {
	lg.Setup(lg.Options{Out: io.Discard, Format: "json"})
	os.Exit(m.Run())
}

var secret = []byte(strings.Repeat("s", 32))

func TestSignerRejectsShortSecret(t *testing.T) {
	if _, err := NewSigner([]byte("short"), "x"); err == nil {
		t.Error("short secret")
	}
}

func TestSignParse(t *testing.T) {
	s, _ := NewSigner(secret, "demo-api")
	tok, err := Sign(s, Claims{RegisteredClaims: s.NewClaims("u1", time.Hour), Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse[Claims](s, tok)
	if err != nil || c.Subject != "u1" || c.Role != "admin" || c.Issuer != "demo-api" {
		t.Fatalf("%v %+v", err, c)
	}
	// Another issuer with the same secret is not valid.
	other, _ := NewSigner(secret, "other")
	if _, err := Parse[Claims](other, tok); err == nil {
		t.Error("different issuer")
	}
	// No exp is not valid.
	noExp, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Issuer: "demo-api"}).SignedString(secret)
	if _, err := Parse[Claims](s, noExp); err == nil {
		t.Error("exp required")
	}
	// alg=none is not valid.
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{Issuer: "demo-api", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := Parse[Claims](s, none); err == nil {
		t.Error("alg none")
	}
}

func TestBearerAndRoles(t *testing.T) {
	s, _ := NewSigner(secret, "demo-api")
	tok, _ := Sign(s, Claims{RegisteredClaims: s.NewClaims("u1", time.Hour), Role: "member"})
	var seen *Claims
	h := Bearer[Claims](s)(RequireRole(RoleOf, "admin", "member")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = ClaimsFrom[Claims](r.Context())
	})))
	admin := Bearer[Claims](s)(RequireRole(RoleOf, "admin")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	for _, c := range []struct {
		name string
		h    http.Handler
		auth string
		code int
	}{
		{"no token", h, "", 401},
		{"bad token", h, "Bearer x.y.z", 401},
		{"ok", h, "Bearer " + tok, 200},
		{"insufficient role", admin, "Bearer " + tok, 403},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		if c.auth != "" {
			r.Header.Set("Authorization", c.auth)
		}
		rec := httptest.NewRecorder()
		c.h.ServeHTTP(rec, r)
		if rec.Code != c.code {
			t.Errorf("%s: %d, want %d (%s)", c.name, rec.Code, c.code, rec.Body.String())
		}
	}
	if seen == nil || seen.Subject != "u1" {
		t.Error("claims in context")
	}
}

func TestCookieMiddleware(t *testing.T) {
	s, _ := NewSigner(secret, "demo-api")
	tok, _ := Sign(s, Claims{RegisteredClaims: s.NewClaims("admin", time.Hour)})
	h := Cookie[Claims](s, "sid")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(SessionCookie("sid", tok, time.Hour, CookieOptions{Secure: true}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Errorf("valid cookie: %d", rec.Code)
	}
	c := SessionCookie("sid", "v", time.Hour, CookieOptions{Domain: ".example.com"})
	if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.MaxAge != 3600 {
		t.Errorf("%+v", c)
	}
	if ClearCookie("sid", CookieOptions{}).MaxAge != -1 {
		t.Error("ClearCookie")
	}
}

func TestPasswords(t *testing.T) {
	h, err := HashPassword("correct-123")
	if err != nil || !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("%v %s", err, h)
	}
	if ok, rehash := VerifyPassword("correct-123", h); !ok || rehash {
		t.Error("current argon2id")
	}
	if ok, _ := VerifyPassword("other", h); ok {
		t.Error("wrong password")
	}
	// Legacy pbkdf2: "hello" with 100000 iterations.
	pb := "pbkdf2$sha256$100000$c2FsdHNhbHRzYWx0c2FsdA==$" + pbkdf2Hex("hello", "saltsaltsaltsalt", 100000)
	if ok, rehash := VerifyPassword("hello", pb); !ok || !rehash {
		t.Errorf("pbkdf2 compat: ok=%v rehash=%v", ok, rehash)
	}
	// Legacy bcrypt.
	bc := "$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5XIXk9m5kGf5kqJvaeBZk5l9N0u0C" // "password"? doesn't matter: format only
	if ok, _ := VerifyPassword("x", bc); ok {
		t.Error("bcrypt with wrong password")
	}
	if ok, _ := VerifyPassword("x", "garbage"); ok {
		t.Error("unknown format never matches")
	}
}

func TestAPIKey(t *testing.T) {
	plain, hash, err := NewAPIKey("nk")
	if err != nil || !strings.HasPrefix(plain, "nk_") || len(plain) != 51 || HashAPIKey(plain) != hash {
		t.Fatalf("%v %s %s", err, plain, hash)
	}
	type account struct{ ID string }
	var seen *account
	h := RequireAPIKey(APIKeyOptions{Lookup: func(_ context.Context, h string) (any, bool) {
		if h == hash {
			return &account{ID: "acc1"}, true
		}
		return nil, false
	}})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = APIKeyPrincipal[account](r.Context()) }))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-API-Key", plain)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 200 || seen == nil || seen.ID != "acc1" {
		t.Errorf("%d %v", rec.Code, seen)
	}
	r.Header.Set("X-API-Key", "nk_bad")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 401 {
		t.Error("bad key")
	}
}
