package httpapi

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// xForwardedForHeader is the canonical X-Forwarded-For header name.
const xForwardedForHeader = "X-Forwarded-For"

// xForwardedProtoHeader is the canonical X-Forwarded-Proto header name.
const xForwardedProtoHeader = "X-Forwarded-Proto"

// trustedProxy returns middleware that derives the real client IP from the
// X-Forwarded-For header, but only when the immediate peer (the TCP
// RemoteAddr) falls within one of the trusted proxy prefixes. Gating on the
// peer prevents a client connecting directly from spoofing its address with a
// forged header.
//
// The resolved IP is stored in the request context and read by clientIPFrom;
// the forwarded scheme (X-Forwarded-Proto) is recovered the same way and read
// by schemeFrom. When no prefixes are configured, or the peer is not trusted,
// the middleware is a no-op and the peer address and TLS state remain
// authoritative.
func trustedProxy(prefixes []netip.Prefix) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(prefixes) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			peer, ok := peerAddr(r.RemoteAddr)
			if !ok || !inAnyPrefix(peer, prefixes) {
				next.ServeHTTP(w, r)
				return
			}

			ctx := r.Context()
			if ip, ok := clientFromXFF(r.Header[xForwardedForHeader], prefixes); ok {
				ctx = context.WithValue(ctx, clientIPCtxKey, ip)
			}
			if scheme, ok := forwardedProto(r.Header[xForwardedProtoHeader]); ok {
				ctx = context.WithValue(ctx, clientSchemeCtxKey, scheme)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// peerAddr extracts the connecting peer's IP from a RemoteAddr string, folding
// v4-mapped IPv6 addresses to plain IPv4 and stripping any zone so prefix
// containment checks behave consistently.
func peerAddr(remoteAddr string) (netip.Addr, bool) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr // RemoteAddr may already be a bare IP (e.g. in tests).
	}
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip.Unmap().WithZone(""), true
}

// clientFromXFF walks the merged X-Forwarded-For chain right-to-left, skipping
// entries within the trusted prefixes, and returns the first untrusted IP — the
// real client. An unparseable entry aborts the walk (fail-closed) so nothing
// left of garbage is trusted.
func clientFromXFF(headers []string, prefixes []netip.Prefix) (netip.Addr, bool) {
	var found netip.Addr
	walkXFF(headers, func(entry string) bool {
		ip, err := netip.ParseAddr(entry)
		if err != nil {
			return true // fail-closed; leave found unset
		}
		ip = ip.Unmap().WithZone("")
		if inAnyPrefix(ip, prefixes) {
			return false // trusted hop; keep walking left
		}
		found = ip
		return true
	})
	if !found.IsValid() {
		return netip.Addr{}, false
	}
	return found, true
}

// walkXFF walks the entries of the merged X-Forwarded-For chain right-to-left,
// invoking visit on each trimmed non-empty entry. visit returns true to stop.
// Multiple headers are merged per RFC 7239 so a duplicate header cannot be used
// to pick which value the trust logic sees.
func walkXFF(headers []string, visit func(entry string) bool) {
	for hi := len(headers) - 1; hi >= 0; hi-- {
		h := headers[hi]
		for h != "" {
			var v string
			if i := strings.LastIndexByte(h, ','); i >= 0 {
				v, h = h[i+1:], h[:i]
			} else {
				v, h = h, ""
			}
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if visit(v) {
				return
			}
		}
	}
}

// inAnyPrefix reports whether ip falls within any of the given prefixes.
func inAnyPrefix(ip netip.Addr, prefixes []netip.Prefix) bool {
	for _, p := range prefixes {
		if p.Contains(ip) {
			return true
		}
	}
	return false
}

// clientIPFrom returns the trusted-proxy-resolved client IP stored by
// trustedProxy, or false when none was set (no trusted proxy, or untrusted
// peer).
func clientIPFrom(ctx context.Context) (netip.Addr, bool) {
	ip, ok := ctx.Value(clientIPCtxKey).(netip.Addr)
	return ip, ok
}

// forwardedProto returns the originating scheme from the X-Forwarded-Proto
// header set by a trusted proxy. The leftmost entry is the original client's
// scheme; only the recognized "http"/"https" values are accepted, and an
// unrecognized first entry fails closed so a forged value is ignored.
func forwardedProto(headers []string) (string, bool) {
	for _, h := range headers {
		for _, entry := range strings.Split(h, ",") {
			entry = strings.ToLower(strings.TrimSpace(entry))
			if entry == "" {
				continue
			}
			if entry == "http" || entry == "https" {
				return entry, true
			}
			return "", false // first real entry is unrecognized
		}
	}
	return "", false
}

// schemeFrom returns the trusted-proxy-resolved request scheme stored by
// trustedProxy, or false when none was set (no trusted proxy, or untrusted
// peer).
func schemeFrom(ctx context.Context) (string, bool) {
	scheme, ok := ctx.Value(clientSchemeCtxKey).(string)
	return scheme, ok
}
