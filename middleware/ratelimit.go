package middleware

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/g-studio-labs/gokit/response"
)

// RateLimitOptions configures RateLimit.
type RateLimitOptions struct {
	// Limit requests per Window and per key. E.g. 20 per minute.
	Limit  int
	Window time.Duration
	// Key groups requests; by default, the client IP (ClientIP). For a login,
	// the IP; for an API key, the key.
	Key func(r *http.Request) string
	// Skip exempts requests (e.g. those from an internal API key).
	Skip func(r *http.Request) bool
}

// RateLimit is an in-memory per-key limit (token bucket). It works for one
// process; with several replicas each keeps its own count, which is usually
// enough to curb abuse. For an exact limit across replicas, use the database
// or Redis. It responds 429 with the envelope and Retry-After.
//
// For a persistent login lockout (5 failures in 15 min per IP) the house
// uses a database table; this is a complement,
// not a substitute.
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
	rate    float64 // tokens per second
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
