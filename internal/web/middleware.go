package web

import (
	"net/http"

	"github.com/aldersfors/matlistan/internal/i18n"
)

const _csp = "default-src 'self'; img-src 'self' data:; style-src 'self'; script-src 'self'; " +
	"base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", _csp)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func withCatalog(c *i18n.Catalog, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(i18n.WithCatalog(r.Context(), c)))
	})
}
