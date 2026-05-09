package web

import (
	"crypto/subtle"
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
// constant-time match against the active server token. On a positive
// match it pings the idle-rotate watcher (K-33) so the rotation timer
// resets on every authorized request.
func (s *Server) hasValidSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookieName)
	if err != nil || c.Value == "" {
		return false
	}
	current := s.currentToken()
	if subtle.ConstantTimeCompare([]byte(c.Value), []byte(current)) != 1 {
		return false
	}
	s.markAuthorized()
	return true
}

// setSessionCookie writes the authenticated cookie. Secure is set only
// when the bound address is non-loopback because browsers reject Secure
// cookies on plain HTTP.
func (s *Server) setSessionCookie(w http.ResponseWriter) {
	// #nosec G124 -- Secure conditional on non-loopback by design;
	// HTTP loopback rejects Secure cookies. HttpOnly + SameSite=Strict
	// are unconditional.
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    s.currentToken(),
		Path:     "/",
		MaxAge:   sessionMaxAgeSeconds,
		HttpOnly: true,
		Secure:   !s.loopback,
		SameSite: http.SameSiteStrictMode,
	})
}

// clearSessionCookie expires the session cookie on the client.
func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	// #nosec G124 -- same Secure-on-non-loopback rationale as
	// setSessionCookie above.
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

