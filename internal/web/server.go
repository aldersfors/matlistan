// Package web serves the Matlistan UI.
package web

import (
	"context"
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"

	"github.com/jalet/matlistan/internal/household"
	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/recipes"
	"github.com/jalet/matlistan/internal/theme"
)

//go:embed static
var _static embed.FS

// Authenticator guards app routes and serves /auth/*.
type Authenticator interface {
	Routes(mux *http.ServeMux)
	Require(next http.Handler) http.Handler
}

// Store is what the web layer needs from persistence.
type Store interface {
	Ping(ctx context.Context) error
	ListMembers(ctx context.Context) ([]household.Member, error)
	GetMember(ctx context.Context, id int64) (household.Member, error)
	CreateMember(ctx context.Context, m household.Member) (int64, error)
	UpdateMember(ctx context.Context, m household.Member) error
	ArchiveMember(ctx context.Context, id int64) error
	LinkMember(ctx context.Context, id int64, subject string) error
	GetSettings(ctx context.Context) (household.Settings, error)
	UpdateSettings(ctx context.Context, s household.Settings) error
	ListStaples(ctx context.Context) ([]household.Staple, error)
	AddStaple(ctx context.Context, name string) error
	RemoveStaple(ctx context.Context, id int64) error
	ListRecipes(ctx context.Context, lang i18n.Locale, q string) ([]recipes.Summary, error)
	GetRecipe(ctx context.Context, id int64) (recipes.Recipe, error)
	CreateRecipe(ctx context.Context, r recipes.Recipe) (int64, error)
	UpdateRecipe(ctx context.Context, r recipes.Recipe) error
	ArchiveRecipe(ctx context.Context, id int64) error
}

// Deps are the server's collaborators.
type Deps struct {
	Catalog *i18n.Catalog
	Theme   theme.Theme
	Auth    Authenticator
	Store   Store
	Now     func() time.Time // in the configured location
	Log     zerolog.Logger
}

type server struct{ Deps }

// New builds the HTTP handler. Public: /healthz, /static/*, /auth/*.
// Everything else requires a session.
func New(d Deps) http.Handler {
	if d.Catalog == nil || d.Auth == nil || d.Store == nil || d.Now == nil {
		panic("invariant violated: web.New needs catalog, auth, store and clock")
	}
	s := &server{d}
	static, err := fs.Sub(_static, "static")
	if err != nil {
		panic("invariant violated: embedded static: " + err.Error())
	}
	themeCSS := d.Theme.CSS()

	mux := http.NewServeMux()
	d.Auth.Routes(mux)
	mux.Handle("GET /static/", http.StripPrefix("/static/", noListing(http.FileServerFS(static))))
	mux.HandleFunc("GET /static/theme.css", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(themeCSS)
	})
	mux.HandleFunc("GET /healthz", s.healthz)

	app := http.NewServeMux()
	app.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/week", http.StatusSeeOther)
	})
	app.HandleFunc("GET /week", s.week)
	app.HandleFunc("GET /family", s.family)
	app.HandleFunc("GET /family/new", s.newMember)
	app.HandleFunc("POST /family", s.createMember)
	app.HandleFunc("GET /family/{id}/edit", s.editMember)
	app.HandleFunc("POST /family/{id}", s.updateMember)
	app.HandleFunc("POST /family/{id}/archive", s.archiveMember)
	app.HandleFunc("POST /family/{id}/me", s.linkMember)
	app.HandleFunc("GET /settings", s.settings)
	app.HandleFunc("POST /settings", s.saveSettings)
	app.HandleFunc("POST /settings/staples", s.addStaple)
	app.HandleFunc("POST /settings/staples/{id}/delete", s.removeStaple)
	app.HandleFunc("/", s.notFound)
	// Cross-origin protection covers every app request, so later POST handlers need no
	// per-form token.
	mux.Handle("/", http.NewCrossOriginProtection().Handler(d.Auth.Require(app)))

	return withCatalog(d.Catalog, securityHeaders(mux))
}

func (s *server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.Store.Ping(ctx); err != nil {
		s.Log.Warn().Err(err).Msg("healthz: database")
		http.Error(w, "database unavailable", http.StatusServiceUnavailable)
		return
	}
	_, _ = io.WriteString(w, "ok\n")
}

// Metrics serves Prometheus metrics. It runs on its own internal listener, never on the
// public handler, so the HTTPRoute cannot expose it.
func Metrics() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.Handler())
	return mux
}

// noListing answers 404 for directory paths, so the file server never lists the tree.
func noListing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "" || strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
