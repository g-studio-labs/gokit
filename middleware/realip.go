package middleware

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type ipKey struct{}

// Rangos publicados por Cloudflare (https://www.cloudflare.com/ips), 24-oct-2026.
var cloudflareRanges = mustPrefixes(
	"173.245.48.0/20", "103.21.244.0/22", "103.22.200.0/22", "103.31.4.0/22",
	"141.101.64.0/18", "108.162.192.0/18", "190.93.240.0/20", "188.114.96.0/20",
	"197.234.240.0/22", "198.41.128.0/17", "162.158.0.0/15", "104.16.0.0/13",
	"104.24.0.0/14", "172.64.0.0/13", "131.0.72.0/22",
	"2400:cb00::/32", "2606:4700::/32", "2803:f800::/32", "2405:b500::/32",
	"2405:8100::/32", "2a06:98c0::/29", "2c0f:f248::/32",
)

// CloudflareRanges devuelve los rangos de Cloudflare (copia).
func CloudflareRanges() []netip.Prefix { return append([]netip.Prefix(nil), cloudflareRanges...) }

// PrivateRanges son las redes privadas y loopback: donde viven Caddy, Envoy y
// el resto del stack cuando el API no se publica.
func PrivateRanges() []netip.Prefix {
	return mustPrefixes("10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "fc00::/7", "::1/128")
}

// RealIP decide la IP del visitante sin dejar que la falsee, con la lógica que
// santa-travel ya probó detrás de Caddy, Envoy y Cloudflare:
//
//  1. El par es quien abrió el socket. Si está en trusted (por defecto, las
//     redes privadas: el API no se publica y solo el proxy llega), el visitante
//     es el ÚLTIMO X-Forwarded-For: lo que viene antes lo escribió el cliente y
//     puede ser cualquier cosa. Si el par no es de confianza, X-Forwarded-For
//     se ignora por completo.
//  2. Si ese par es Cloudflare, la IP del visitante es CF-Connecting-IP. Si no,
//     ese header también se ignora.
//
// Sin proxy delante no uses este middleware: ClientIP cae al RemoteAddr.
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

// ClientIP devuelve la IP que dejó RealIP, o el host de RemoteAddr si no se usó.
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
