package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cartine/thimble/internal/web"
)

func TestNoStoreMiddlewareSetsHeaders(t *testing.T) {
	st := newTestStore(t)
	server := web.New(st, "test-token", true)
	mux := http.NewServeMux()
	server.Routes(mux)
	handler := web.NoStoreMiddleware(mux)

	cookie := loginAndExtractCookie(t, handler, "test-token")
	assertNoStoreHeaders(t, handler, "GET", "/login", nil)
	assertNoStoreHeaders(t, handler, "GET", "/", cookie)
}

func assertNoStoreHeaders(t *testing.T, h http.Handler, method, target string,
	c *http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, target, nil)
	if c != nil {
		req.AddCookie(c)
	}
	h.ServeHTTP(rec, req)
	cc := rec.Header().Get("Cache-Control")
	if !strings.Contains(cc, "no-store") || !strings.Contains(cc, "no-cache") ||
		!strings.Contains(cc, "must-revalidate") {
		t.Fatalf("%s %s Cache-Control = %q; want all of "+
			"no-store/no-cache/must-revalidate", method, target, cc)
	}
	if pragma := rec.Header().Get("Pragma"); pragma != "no-cache" {
		t.Fatalf("%s %s Pragma = %q, want no-cache", method, target, pragma)
	}
}
