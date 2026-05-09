package web

import (
	"net"
	"net/http"
	"strings"
)

// HostGuard rejects requests whose Host header is not in the allowlist.
// This is the K-31 DNS-rebinding defense: even if an attacker resolves
// a malicious name to 127.0.0.1, the request is dropped before any
// handler runs.
type HostGuard struct {
	hosts map[string]struct{}
}

// NewHostGuard builds a guard from an explicit list of allowed
// authorities. Every entry is normalised through normalizeHost so the
// caller can pass them in any case and with optional ports. The result
// is non-nil; an empty list means "deny everything", which is fine —
// runWeb always supplies the loopback names plus the bind address.
func NewHostGuard(authorities []string) *HostGuard {
	g := &HostGuard{hosts: map[string]struct{}{}}
	for _, raw := range authorities {
		for _, n := range expandHost(raw) {
			g.hosts[n] = struct{}{}
		}
	}
	return g
}

// Middleware wraps next with a 400-on-mismatch host guard. Requests
// without a Host header (older HTTP/1.0) are rejected.
func (g *HostGuard) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !g.allow(r.Host) {
			http.Error(w, "host not allowed", http.StatusBadRequest)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// allow reports whether host is on the allowlist. Comparison is
// case-insensitive and tolerant of a single trailing dot (e.g.,
// "localhost.").
func (g *HostGuard) allow(host string) bool {
	if host == "" {
		return false
	}
	for _, candidate := range expandHost(host) {
		if _, ok := g.hosts[candidate]; ok {
			return true
		}
	}
	return false
}

// expandHost normalises one user-supplied authority into the matching
// keys we store. It returns up to two entries: the host with port and
// the host alone, both lower-cased and with any trailing dot stripped.
func expandHost(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		host, port = raw, ""
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	host = strings.TrimPrefix(strings.TrimSuffix(host, "]"), "[")
	if host == "" {
		return nil
	}
	if port == "" {
		return []string{host}
	}
	withPort := net.JoinHostPort(host, port)
	return []string{withPort, host}
}

// LoopbackAuthorities returns the canonical local-only allowlist:
// 127.0.0.1, ::1, and localhost — each both with and without the
// configured port. Callers append --addr's host and any --allow-host
// entries on top.
func LoopbackAuthorities(port string) []string {
	bases := []string{"127.0.0.1", "::1", "localhost"}
	out := make([]string, 0, len(bases)*2)
	for _, h := range bases {
		out = append(out, h)
		if port != "" {
			out = append(out, net.JoinHostPort(h, port))
		}
	}
	return out
}
