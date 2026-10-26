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

	"github.com/RomanshkVolkov/gokit/lg"
)

func TestMain(m *testing.M) {
	lg.Setup(lg.Options{Out: io.Discard, Format: "json"})
	os.Exit(m.Run())
}

var secret = []byte(strings.Repeat("s", 32))

func TestSignerRechazaSecretoCorto(t *testing.T) {
	if _, err := NewSigner([]byte("corto"), "x"); err == nil {
		t.Error("secreto corto")
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
	// Otro emisor con el mismo secreto no vale.
	other, _ := NewSigner(secret, "otro")
	if _, err := Parse[Claims](other, tok); err == nil {
		t.Error("emisor distinto")
	}
	// Sin exp no vale.
	noExp, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.RegisteredClaims{Issuer: "demo-api"}).SignedString(secret)
	if _, err := Parse[Claims](s, noExp); err == nil {
		t.Error("exp obligatorio")
	}
	// alg=none no vale.
	none, _ := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.RegisteredClaims{Issuer: "demo-api", ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if _, err := Parse[Claims](s, none); err == nil {
		t.Error("alg none")
	}
}

func TestBearerYRoles(t *testing.T) {
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
		{"sin token", h, "", 401},
		{"token malo", h, "Bearer x.y.z", 401},
		{"ok", h, "Bearer " + tok, 200},
		{"rol insuficiente", admin, "Bearer " + tok, 403},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		if c.auth != "" {
			r.Header.Set("Authorization", c.auth)
		}
		rec := httptest.NewRecorder()
		c.h.ServeHTTP(rec, r)
		if rec.Code != c.code {
			t.Errorf("%s: %d, quería %d (%s)", c.name, rec.Code, c.code, rec.Body.String())
		}
	}
	if seen == nil || seen.Subject != "u1" {
		t.Error("claims en el contexto")
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
		t.Errorf("cookie válida: %d", rec.Code)
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
	h, err := HashPassword("correcta-123")
	if err != nil || !strings.HasPrefix(h, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("%v %s", err, h)
	}
	if ok, rehash := VerifyPassword("correcta-123", h); !ok || rehash {
		t.Error("argon2id actual")
	}
	if ok, _ := VerifyPassword("otra", h); ok {
		t.Error("contraseña incorrecta")
	}
	// pbkdf2 de santa-travel: "hola" con 100000 iteraciones.
	pb := "pbkdf2$sha256$100000$c2FsdHNhbHRzYWx0c2FsdA==$" + pbkdf2Hex("hola", "saltsaltsaltsalt", 100000)
	if ok, rehash := VerifyPassword("hola", pb); !ok || !rehash {
		t.Errorf("pbkdf2 compat: ok=%v rehash=%v", ok, rehash)
	}
	// bcrypt de crm.
	bc := "$2a$10$7EqJtq98hPqEX7fNZaFWoOhi5XIXk9m5kGf5kqJvaeBZk5l9N0u0C" // "password"? no importa: solo formato
	if ok, _ := VerifyPassword("x", bc); ok {
		t.Error("bcrypt con contraseña incorrecta")
	}
	if ok, _ := VerifyPassword("x", "basura"); ok {
		t.Error("formato desconocido nunca coincide")
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
	r.Header.Set("X-API-Key", "nk_mala")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 401 {
		t.Error("llave mala")
	}
}
