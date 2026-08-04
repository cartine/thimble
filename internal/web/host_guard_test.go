package web_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cartine/thimble/internal/web"
)

func TestHostGuardAllowsLoopbackAndRejectsForeign(t *testing.T) {
	cases := []struct {
		name       string
		host       string
		wantStatus int
	}{
		{"127.0.0.1 with port", "127.0.0.1:8787", http.StatusOK},
		{"localhost lowercase", "localhost:8787", http.StatusOK},
		{"localhost uppercase", "LOCALHOST:8787", http.StatusOK},
		{"localhost trailing dot", "localhost.:8787", http.StatusOK},
		{"ipv6 brackets", "[::1]:8787", http.StatusOK},
		{"loopback no port", "127.0.0.1", http.StatusOK},
		{"foreign rebound", "evil.example.com:8787", http.StatusBadRequest},
		{"foreign no port", "attacker.test", http.StatusBadRequest},
		{"empty host", "", http.StatusBadRequest},
	}
	guard := web.NewHostGuard(web.LoopbackAuthorities("8787"))
	handler := guard.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.Host = c.host
			handler.ServeHTTP(rec, req)
			if rec.Code != c.wantStatus {
				t.Fatalf("Host=%q status = %d, want %d body=%q",
					c.host, rec.Code, c.wantStatus, rec.Body.String())
			}
		})
	}
}

func TestHostGuardCustomAllowList(t *testing.T) {
	guard := web.NewHostGuard(append(
		web.LoopbackAuthorities("8787"),
		"foo.local:8787",
	))
	handler := guard.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "FOO.LOCAL:8787"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("custom-host status = %d, want 200; body=%q",
			rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req.Host = "bar.local:8787"
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("disallowed-host status = %d, want 400", rec.Code)
	}
	if rec.Body.String() != "host not allowed\n" {
		t.Fatalf("rejection body = %q, want %q",
			rec.Body.String(), "host not allowed\n")
	}
}

func TestHostGuardRejectsCrossOriginWrites(t *testing.T) {
	cases := []struct {
		name       string
		method     string
		fetchSite  string
		origin     string
		wantStatus int
	}{
		{"read needs no origin", http.MethodGet, "", "", http.StatusOK},
		{"same origin metadata", http.MethodPost, "same-origin", "", http.StatusOK},
		{"browser initiated metadata", http.MethodPost, "none", "", http.StatusOK},
		{"same site is not same origin", http.MethodPost, "same-site", "", http.StatusForbidden},
		{"cross site metadata", http.MethodPost, "cross-site", "", http.StatusForbidden},
		{"exact origin fallback", http.MethodPost, "", "http://localhost:8787", http.StatusOK},
		{"different port fallback", http.MethodPost, "", "http://localhost:3000", http.StatusForbidden},
		{"missing metadata", http.MethodPost, "", "", http.StatusForbidden},
	}
	guard := web.NewHostGuard(web.LoopbackAuthorities("8787"))
	handler := guard.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, "http://localhost:8787/secret", nil)
			req.Host = "localhost:8787"
			req.Header.Set("Sec-Fetch-Site", tc.fetchSite)
			req.Header.Set("Origin", tc.origin)
			handler.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d body=%q", rec.Code, tc.wantStatus,
					rec.Body.String())
			}
		})
	}
}
