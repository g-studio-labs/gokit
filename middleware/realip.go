package middleware

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type ipKey struct{}

// Ranges published by Cloudflare (https://www.cloudflare.com/ips), 2026-10-24.
var cloudflareRanges = mustPrefixes(
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
	"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
	"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
	"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
)

// CloudflareRanges returns Cloudflare's ranges (a copy).
func CloudflareRanges() []netip.Prefix { return append([]netip.Prefix(nil), cloudflareRanges...) }

// PrivateRanges are the private and loopback networks: where Caddy, Envoy and
// the rest of the stack live when the API is not published.
func PrivateRanges() []netip.Prefix {
	return mustPrefixes("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "fc00::/7", "::1/128")
}

// RealIP determines the visitor's IP without letting them spoof it, with the
// logic we already proved behind Caddy, Envoy and Cloudflare:
//
//  1. The peer is whoever opened the socket. If it is in trusted (by default,
//     the private networks: the API is not published and only the proxy reaches
//     it), the visitor is the LAST X-Forwarded-For: anything before it was written
//     by the client and can be anything. If the peer is not trusted,
//     X-Forwarded-For is ignored entirely.
//  2. If that peer is Cloudflare, the visitor's IP is CF-Connecting-IP. If not,
//     that header is ignored too.
//
// Without a proxy in front, do not use this middleware: ClientIP falls back to RemoteAddr.
func RealIP(trusted ...netip.Prefix) Middleware {
	if len(trusted) == 0 {
		trusted = PrivateRanges()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := realIP(r, trusted)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ipKey{}, ip)))
		})
	}
}

// ClientIP returns the IP stored by RealIP, or the RemoteAddr host if it was not used.
func ClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(ipKey{}).(string); ok && ip != "" {
		return ip
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func realIP(r *http.Request, trusted []netip.Prefix) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	peer, ok := parseIP(host)
	if !ok {
		return ""
	}
	if contains(trusted, peer) {
		if xff := strings.Join(r.Header.Values("X-Forwarded-For"), ","); xff != "" {
			parts := strings.Split(xff, ",")
			if p, ok := parseIP(parts[len(parts)-1]); ok {
				peer = p
			}
		}
	}
	if contains(cloudflareRanges, peer) {
		if cf, ok := parseIP(r.Header.Get("CF-Connecting-IP")); ok {
			return cf.String()
		}
	}
	return peer.String()
}

func parseIP(s string) (netip.Addr, bool) {
	ip, err := netip.ParseAddr(strings.TrimSpace(s))
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap(), true
}

func contains(ps []netip.Prefix, ip netip.Addr) bool {
	for _, p := range ps {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

func mustPrefixes(cidrs ...string) []netip.Prefix {
	out := make([]netip.Prefix, 0, len(cidrs))
	for _, c := range cidrs {
		out = append(out, netip.MustParsePrefix(c))
	}
	return out
}
