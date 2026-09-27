package web

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/theme"
)

type fakeAuth struct{ signedIn bool }

func (fakeAuth) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
}

func (f fakeAuth) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !f.signedIn {
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func newServer(t *testing.T, l i18n.Locale, signedIn bool, db fakeDB) http.Handler {
	t.Helper()
	c, err := i18n.Load(l)
	if err != nil {
		t.Fatal(err)
	}
	sthlm, _ := time.LoadLocation("Europe/Stockholm")
	return New(Deps{Catalog: c, Auth: fakeAuth{signedIn: signedIn}, DB: db,
		Now: func() time.Time { return time.Date(2026, 9, 27, 8, 0, 0, 0, sthlm) },
		Log: zerolog.Nop()})
}

func get(t *testing.T, h http.Handler, path string) (*http.Response, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil))
	res := rec.Result()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func TestWeekPageInSwedish(t *testing.T) {
	res, body := get(t, newServer(t, i18n.SV, true, fakeDB{}), "/week")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	for _, want := range []string{`lang="sv"`, "Vecka 40", "28 sep till 4 okt", "Mån", "Sön",
		"Inte planerad", "day-1", "day-7"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q", want)
		}
	}
}

func TestWeekPageDefaultsToEnglish(t *testing.T) {
	_, body := get(t, newServer(t, i18n.EN, true, fakeDB{}), "/week")
	for _, want := range []string{`lang="en"`, "Week 40", "Sep 28 to Oct 4", "Mon", "Not planned"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q", want)
		}
	}
}

func TestRootRedirectsToWeek(t *testing.T) {
	res, _ := get(t, newServer(t, i18n.EN, true, fakeDB{}), "/")
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/week" {
		t.Fatalf("status %d location %q", res.StatusCode, res.Header.Get("Location"))
	}
}

// Review focus 5: app pages need a session; only a fixed set of paths is public.
func TestAppPagesRequireSignIn(t *testing.T) {
	h := newServer(t, i18n.EN, false, fakeDB{})
	for _, p := range []string{"/", "/week", "/anything"} {
		res, _ := get(t, h, p)
		if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/auth/login" {
			t.Errorf("%s: status %d location %q", p, res.StatusCode, res.Header.Get("Location"))
		}
	}
	for _, p := range []string{"/healthz", "/static/app.css", "/static/theme.css",
		"/auth/login"} {
		if res, _ := get(t, h, p); res.StatusCode >= 300 {
			t.Errorf("%s: status %d, want public", p, res.StatusCode)
		}
	}
}

// Review focus 3: a down database makes /healthz fail fast.
func TestHealthz(t *testing.T) {
	if res, _ := get(t, newServer(t, i18n.EN, false, fakeDB{}), "/healthz"); res.StatusCode != 200 {
		t.Errorf("up: %d", res.StatusCode)
	}
	down := newServer(t, i18n.EN, false, fakeDB{err: errors.New("down")})
	if res, _ := get(t, down, "/healthz"); res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("down: %d", res.StatusCode)
	}
}

func TestSecurityHeaders(t *testing.T) {
	res, _ := get(t, newServer(t, i18n.EN, true, fakeDB{}), "/week")
	csp := res.Header.Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") || strings.Contains(csp, "unsafe-inline") {
		t.Errorf("CSP = %q", csp)
	}
	if res.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Error("missing nosniff")
	}
}

func TestThemeCSSServed(t *testing.T) {
	c, _ := i18n.Load(i18n.EN)
	th, err := theme.Parse([]byte(`light: {page: "#ffffff"}`))
	if err != nil {
		t.Fatal(err)
	}
	h := New(Deps{Catalog: c, Theme: th, Auth: fakeAuth{}, DB: fakeDB{},
		Now: time.Now, Log: zerolog.Nop()})
	res, body := get(t, h, "/static/theme.css")
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/css") ||
		!strings.Contains(body, "--ml-page: #ffffff") {
		t.Fatalf("type %q body %q", res.Header.Get("Content-Type"), body)
	}
}

// Metrics go to a separate internal listener, so the public route never serves them.
func TestMetricsAreNotOnThePublicHandler(t *testing.T) {
	res, _ := get(t, newServer(t, i18n.EN, false, fakeDB{}), "/metrics")
	if res.StatusCode == http.StatusOK {
		t.Fatal("/metrics served on the public handler")
	}
	rec := httptest.NewRecorder()
	Metrics().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/metrics", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "go_goroutines") {
		t.Fatalf("metrics handler: %d", rec.Code)
	}
}

func TestStaticHasNoDirectoryListing(t *testing.T) {
	h := newServer(t, i18n.EN, false, fakeDB{})
	for _, p := range []string{"/static/", "/static/fonts/"} {
		if res, body := get(t, h, p); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d body %.60q", p, res.StatusCode, body)
		}
	}
}

// State-changing requests from another site are refused before they reach a handler.
func TestCrossSitePostIsRejected(t *testing.T) {
	h := newServer(t, i18n.EN, true, fakeDB{})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/week", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", rec.Code)
	}
}
