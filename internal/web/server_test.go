package web

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/aldersfors/matlistan/internal/auth"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/theme"
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
		ctx := auth.WithSession(r.Context(), auth.Session{Subject: "sub-anna", Name: "Anna", Email: "anna@example.org"})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func newServer(t *testing.T, l i18n.Locale, signedIn bool, st *fakeStore) http.Handler {
	t.Helper()
	c, err := i18n.Load(l)
	if err != nil {
		t.Fatal(err)
	}
	sthlm, _ := time.LoadLocation("Europe/Stockholm")
	return New(Deps{Catalog: c, Auth: fakeAuth{signedIn: signedIn}, Store: st,
		Now:     func() time.Time { return time.Date(2026, 9, 27, 8, 0, 0, 0, sthlm) },
		BaseURL: "https://matlistan.example.lan", Log: zerolog.Nop()})
}

func post(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path,
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
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
	res, body := get(t, newServer(t, i18n.SV, true, newFakeStore()), "/week")
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
	_, body := get(t, newServer(t, i18n.EN, true, newFakeStore()), "/week")
	for _, want := range []string{`lang="en"`, "Week 40", "Sep 28 to Oct 4", "Mon", "Not planned"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q", want)
		}
	}
}

func TestRootRedirectsToWeek(t *testing.T) {
	res, _ := get(t, newServer(t, i18n.EN, true, newFakeStore()), "/")
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/week" {
		t.Fatalf("status %d location %q", res.StatusCode, res.Header.Get("Location"))
	}
}

// Review focus 5: app pages need a session; only a fixed set of paths is public.
func TestAppPagesRequireSignIn(t *testing.T) {
	h := newServer(t, i18n.EN, false, newFakeStore())
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
	if res, _ := get(t, newServer(t, i18n.EN, false, newFakeStore()), "/healthz"); res.StatusCode != 200 {
		t.Errorf("up: %d", res.StatusCode)
	}
	st := newFakeStore()
	st.pingErr = errors.New("down")
	down := newServer(t, i18n.EN, false, st)
	if res, _ := get(t, down, "/healthz"); res.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("down: %d", res.StatusCode)
	}
}

func TestSecurityHeaders(t *testing.T) {
	res, _ := get(t, newServer(t, i18n.EN, true, newFakeStore()), "/week")
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
	h := New(Deps{Catalog: c, Theme: th, Auth: fakeAuth{}, Store: newFakeStore(),
		Now: time.Now, Log: zerolog.Nop()})
	res, body := get(t, h, "/static/theme.css")
	if !strings.HasPrefix(res.Header.Get("Content-Type"), "text/css") ||
		!strings.Contains(body, "--ml-page: #ffffff") {
		t.Fatalf("type %q body %q", res.Header.Get("Content-Type"), body)
	}
}

// Metrics go to a separate internal listener, so the public route never serves them.
func TestMetricsAreNotOnThePublicHandler(t *testing.T) {
	res, _ := get(t, newServer(t, i18n.EN, false, newFakeStore()), "/metrics")
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
	h := newServer(t, i18n.EN, false, newFakeStore())
	for _, p := range []string{"/static/", "/static/fonts/"} {
		if res, body := get(t, h, p); res.StatusCode != http.StatusNotFound {
			t.Errorf("%s: status %d body %.60q", p, res.StatusCode, body)
		}
	}
}

// State-changing requests from another site are refused before they reach a handler.
func TestCrossSitePostIsRejected(t *testing.T) {
	h := newServer(t, i18n.EN, true, newFakeStore())
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/week", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", rec.Code)
	}
}

func TestNavigationMarksTheCurrentPage(t *testing.T) {
	_, body := get(t, newServer(t, i18n.SV, true, newFakeStore()), "/week")
	for _, want := range []string{`href="/recipes"`, `href="/family"`, `href="/settings"`,
		`aria-current="page"`, "Huvudmeny"} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q", want)
		}
	}
}

// Review focus 5: an oversized form is refused with 400.
func TestOversizedFormIsRejected(t *testing.T) {
	h := newServer(t, i18n.EN, true, newFakeStore())
	rec := post(t, h, "/settings", url.Values{"x": {strings.Repeat("a", 70<<10)}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", rec.Code)
	}
}

func itoa(id int64) string { return strconv.FormatInt(id, 10) }

// Values Postgres cannot store (NUL, invalid UTF-8) are refused as a bad request, not a 500.
func TestUnstorableTextIsRejected(t *testing.T) {
	h := newServer(t, i18n.SV, true, newFakeStore())
	for _, raw := range []string{"name=a%00b&birth_year=2014", "name=%FF&birth_year=2014"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/family",
			strings.NewReader(raw))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", raw, rec.Code)
		}
	}
	for _, q := range []string{"%00", "%FF"} {
		if res, _ := get(t, h, "/recipes?q="+q); res.StatusCode != http.StatusBadRequest {
			t.Errorf("q=%s: status %d, want 400", q, res.StatusCode)
		}
	}
}

func postHX(t *testing.T, h http.Handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path,
		strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}
