package api

import "net/http"

// CORSMiddleware allows a single configured browser origin to call this API directly - a
// workaround for the missing BFF proxy layer web-next was meant to have (docs/09-risk-register.md
// R-10). allowedOrigin empty means the middleware is a no-op passthrough: safe default for any
// topology where only a server-side caller (a real BFF, or none yet) ever reaches the gateway.
func CORSMiddleware(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if allowedOrigin == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key, If-Match")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
