package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/RomanshkVolkov/gokit/response"
)

// RateLimitOptions configura RateLimit.
type RateLimitOptions struct {
	// Limit peticiones por Window y por clave. P. ej. 20 por minuto.
	Limit  int
	Window time.Duration
	// Key agrupa peticiones; por defecto, la IP del cliente (ClientIP). Para un
	// login, la IP; para un API key, la llave.
	Key func(r *http.Request) string
	// Skip exime peticiones (p. ej. las de un API key interno).
	Skip func(r *http.Request) bool
}

// RateLimit es un límite por clave en memoria (token bucket). Sirve para un
// proceso; con varias réplicas cada una lleva su cuenta, lo cual suele bastar
// para frenar abuso. Para un límite exacto entre réplicas, usa la base o
// Redis. Responde 429 con el sobre y Retry-After.
//
// Para un bloqueo de login persistente (5 fallos en 15 min por IP) la casa
// usa una tabla en la base (santa-travel: IntentosRepo); esto es complemento,
// no sustituto.
func RateLimit(o RateLimitOptions) Middleware {
	if o.Limit <= 0 {
		o.Limit = 60
	}
	if o.Window <= 0 {
		o.Window = time.Minute
	}
	if o.Key == nil {
		o.Key = ClientIP
	}
	l := &limiter{
		rate:    float64(o.Limit) / o.Window.Seconds(),
		burst:   float64(o.Limit),
		buckets: map[string]*bucket{},
		ttl:     2 * o.Window,
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if o.Skip != nil && o.Skip(r) {
				next.ServeHTTP(w, r)
				return
			}
			ok, retry := l.allow(o.Key(r), time.Now())
			if !ok {
				w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
				response.Fail(w, r, response.TooManyRequests(""))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type bucket struct {
	tokens float64
	last   time.Time
}

type limiter struct {
	mu      sync.Mutex
	rate    float64 // tokens por segundo
	burst   float64
	buckets map[string]*bucket
	ttl     time.Duration
	ops     int
}

func (l *limiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ops++
	if l.ops%1000 == 0 {
		l.sweep(now)
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.burst, b.tokens+now.Sub(b.last).Seconds()*l.rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	return false, time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
}

func (l *limiter) sweep(now time.Time) {
	for k, b := range l.buckets {
		if now.Sub(b.last) > l.ttl {
			delete(l.buckets, k)
		}
	}
}
