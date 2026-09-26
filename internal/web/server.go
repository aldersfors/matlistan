// Package web serves the Matlistan UI.
package web

import (
	"context"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/theme"
)

//go:embed static
var _static embed.FS

// Authenticator guards app routes and serves /auth/*.
type Authenticator interface {
	Routes(mux *http.ServeMux)
	Require(next http.Handler) http.Handler
}

// Pinger reports database health.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Deps are the server's collaborators.
type Deps struct {
	Catalog *i18n.Catalog
	Theme   theme.Theme
	Auth    Authenticator
	DB      Pinger
	Now     func() time.Time // in the configured location
	Log     zerolog.Logger
}

type server struct{ Deps }

// New builds the HTTP handler. Public: /healthz, /metrics, /static/*, /auth/*.
// Everything else requires a session.
func New(d Deps) http.Handler {
	if d.Catalog == nil || d.Auth == nil || d.DB == nil || d.Now == nil {
		panic("invariant violated: web.New needs catalog, auth, db and clock")
	}
	s := &server{d}
	static, err := fs.Sub(_static, "static")
	if err != nil {
		panic("invariant violated: embedded static: " + err.Error())
	}
	themeCSS := d.Theme.CSS()

	mux := http.NewServeMux()
	d.Auth.Routes(mux)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(static)))
	mux.HandleFunc("GET /static/theme.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(themeCSS)
	})
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.Handle("GET /metrics", promhttp.Handler())

	app := http.NewServeMux()
	app.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/week", http.StatusSeeOther)
	})
	app.HandleFunc("GET /week", s.week)
	app.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, i18n.T(r.Context(), "error.not_found"), http.StatusNotFound)
	})
	mux.Handle("/", d.Auth.Require(app))

	return withCatalog(d.Catalog, securityHeaders(mux))
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.DB.Ping(ctx); err != nil {
		s.Log.Warn().Err(err).Msg("healthz: database")
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	_, _ = io.WriteString(w, "ok\n")
}
