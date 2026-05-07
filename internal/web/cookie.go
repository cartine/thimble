package web

import (
	"crypto/subtle"
	"net"
	"net/http"
)

// sessionCookieName is the name of the cookie that proves a browser
// session has authenticated against the printed token. Constant so
// tests and handlers agree.
const sessionCookieName = "thimble_session"

// sessionMaxAgeSeconds keeps a browser session alive for one hour from
// the most recent login. Re-authenticate forces another paste of the
// startup token.
const sessionMaxAgeSeconds = 3600

// hasValidSession reports whether r carries the session cookie with a
// constant-time match against the server token.
func (s *Server) hasValidSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(s.token)) == 1
}

// setSessionCookie writes the authenticated cookie. Secure is set only
// when the bound address is non-loopback because browsers reject Secure
// cookies on plain HTTP.
func (s *Server) setSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    s.token,
		Path:     "/",
		MaxAge:   sessionMaxAgeSeconds,
		HttpOnly: true,
		Secure:   !s.loopback,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearSessionCookie expires the session cookie on the client.
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   !s.loopback,
		SameSite: http.SameSiteStrictMode,
	})
}

// isLoopbackHost is the canonical host check for the Secure-cookie
// decision. A blank host (e.g., ":8787") is treated as loopback because
// Go's default for an empty host on the listener is all interfaces but
// the operator's address bar still routes through 127.0.0.1.
func isLoopbackHost(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
