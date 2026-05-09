package web

import "net/http"

// NoStoreMiddleware sets cache-prevention headers on every response.
// Web pages list namespace keys, recipients, and notice/error query
// values — none should remain in the browser cache when an operator
// walks away from an unlocked laptop. K-32.
func NoStoreMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control",
			"no-store, no-cache, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		next.ServeHTTP(w, r)
	})
}
