package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/g-studio-labs/gokit/lg"
)

var logs bytes.Buffer

func TestMain(m *testing.M) {
	lg.Setup(lg.Options{Out: &logs, Format: "json"})
	os.Exit(m.Run())
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
}

func TestChainOrden(t *testing.T) {
	var order []string
	mk := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, name)
				next.ServeHTTP(w, r)
			})
		}
	}
	Chain(okHandler(), mk("a"), mk("b"), mk("c")).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if strings.Join(order, "") != "abc" {
		t.Errorf("orden %v", order)
	}
}

func TestRequestID(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { seen = lg.RequestID(r.Context()) }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if seen == "" || rec.Header().Get(RequestIDHeader) != seen || len(seen) != 24 {
		t.Errorf("generado: %q", seen)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set(RequestIDHeader, "abc-123")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if seen != "abc-123" {
		t.Error("respeta el del cliente")
	}
	r.Header.Set(RequestIDHeader, "con espacios\n")
	h.ServeHTTP(httptest.NewRecorder(), r)
	if seen == "con espacios\n" {
		t.Error("uno raro se reemplaza")
	}
}

func TestRealIP(t *testing.T) {
	cases := []struct {
		name, remote, xff, cf, want string
	}{
		{"sin proxy", "203.0.113.5:1234", "198.51.100.9", "", "203.0.113.5"},
		{"proxy privado toma el último XFF", "10.0.0.2:1234", "1.1.1.1, 198.51.100.9", "", "198.51.100.9"},
		{"XFF falso desde internet se ignora", "203.0.113.5:1234", "10.0.0.1", "", "203.0.113.5"},
		{"cloudflare tras el proxy", "10.0.0.2:1234", "104.16.1.1", "198.51.100.77", "198.51.100.77"},
		{"CF-Connecting-IP sin ser cloudflare se ignora", "10.0.0.2:1234", "198.51.100.9", "9.9.9.9", "198.51.100.9"},
		{"ipv4 mapeada", "[::ffff:203.0.113.5]:1234", "", "", "203.0.113.5"},
	}
	for _, c := range cases {
		var got string
		h := RealIP()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = ClientIP(r) }))
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		if c.xff != "" {
			r.Header.Set("X-Forwarded-For", c.xff)
		}
		if c.cf != "" {
			r.Header.Set("CF-Connecting-IP", c.cf)
		}
		h.ServeHTTP(httptest.NewRecorder(), r)
		if got != c.want {
			t.Errorf("%s: %q, quería %q", c.name, got, c.want)
		}
	}
}

func TestLoggerRedactaYNivel(t *testing.T) {
	logs.Reset()
	h := Logger(LoggerOptions{})(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/falla" {
			w.WriteHeader(500)
		}
		w.Write([]byte("x"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/a?token=SECRETO&q=1", nil))
	if rec.Header().Get("X-Response-Time") == "" {
		t.Error("X-Response-Time")
	}
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/falla", nil))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/health", nil))
	out := logs.String()
	if strings.Contains(out, "SECRETO") || !strings.Contains(out, "token=REDACTED") {
		t.Errorf("redacción: %s", out)
	}
	if !strings.Contains(out, `"level":"INFO"`) || !strings.Contains(out, `"level":"ERROR"`) {
		t.Errorf("niveles: %s", out)
	}
	if strings.Contains(out, "/health") {
		t.Error("/health no se registra")
	}
	if !strings.Contains(out, `"status":200`) {
		t.Error("status 200 implícito cuando el handler solo escribe")
	}
}

func TestRecover(t *testing.T) {
	h := Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("boom") }))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"error":"internal"`) || strings.Contains(rec.Body.String(), "boom") {
		t.Errorf("%d %s", rec.Code, rec.Body.String())
	}
	defer func() {
		if recover() != http.ErrAbortHandler {
			t.Error("ErrAbortHandler se relanza")
		}
	}()
	Recover(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic(http.ErrAbortHandler) })).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
}

func TestCORS(t *testing.T) {
	h := CORS(CORSOptions{Origins: []string{"https://admin.example.com"}, AllowCredentials: true})(okHandler())
	r := httptest.NewRequest("OPTIONS", "/", nil)
	r.Header.Set("Origin", "https://admin.example.com")
	r.Header.Set("Access-Control-Request-Method", "POST")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 204 || rec.Header().Get("Access-Control-Allow-Origin") != "https://admin.example.com" || rec.Header().Get("Access-Control-Allow-Credentials") != "true" || rec.Header().Get("Vary") != "Origin" {
		t.Errorf("preflight: %d %v", rec.Code, rec.Header())
	}
	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://evil.example.com")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("origen ajeno no recibe headers")
	}
	// "*" con credenciales no se acepta.
	h = CORS(CORSOptions{Origins: []string{"*"}, AllowCredentials: true})(okHandler())
	r = httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Origin", "https://any.example.com")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("* con credenciales se ignora")
	}
	h = CORS(CORSOptions{Origins: []string{"*"}})(okHandler())
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("* sin credenciales sí")
	}
}

func TestAntiCSRF(t *testing.T) {
	h := AntiCSRF([]string{"https://app.example.com"})(okHandler())
	for _, c := range []struct {
		method, origin string
		code           int
	}{{"POST", "https://evil.example.com", 403}, {"POST", "https://app.example.com", 200}, {"POST", "", 200}, {"GET", "https://evil.example.com", 200}} {
		r := httptest.NewRequest(c.method, "/", nil)
		if c.origin != "" {
			r.Header.Set("Origin", c.origin)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if rec.Code != c.code {
			t.Errorf("%s %q → %d, quería %d", c.method, c.origin, rec.Code, c.code)
		}
		if c.code == 403 && !strings.Contains(rec.Body.String(), CodeOriginNotAllowed) {
			t.Error("código del sobre")
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	SecurityHeaders(HeaderOptions{HSTS: true})(okHandler()).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	for _, k := range []string{"X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy", "Content-Security-Policy", "Strict-Transport-Security"} {
		if rec.Header().Get(k) == "" {
			t.Errorf("falta %s", k)
		}
	}
}

func TestMaxBody(t *testing.T) {
	var readErr error
	h := MaxBody(5)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, readErr = io.ReadAll(r.Body) }))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/", strings.NewReader("123456789")))
	if _, ok := readErr.(*http.MaxBytesError); !ok {
		t.Errorf("quería MaxBytesError, fue %v", readErr)
	}
}

func TestRateLimit(t *testing.T) {
	h := RateLimit(RateLimitOptions{Limit: 3, Window: time.Minute})(okHandler())
	codes := []int{}
	for range 5 {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "203.0.113.5:1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		codes = append(codes, rec.Code)
		if rec.Code == 429 && (rec.Header().Get("Retry-After") == "" || !strings.Contains(rec.Body.String(), "rate-limited")) {
			t.Error("429 con Retry-After y código")
		}
	}
	if codes[2] != 200 || codes[3] != 429 {
		t.Errorf("%v", codes)
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.6:1"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Error("otra IP tiene su propia cuenta")
	}
}
