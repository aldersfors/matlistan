package web

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/aldersfors/matlistan/internal/auth"
	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/release"
)

const _testSHA = "abcdef0123456789abcdef0123456789abcdef01"

// buildServer renders with a fixed build, in Stockholm time.
func buildServerWith(t *testing.T, l i18n.Locale, b release.Info) http.Handler {
	t.Helper()
	c, err := i18n.Load(l)
	if err != nil {
		t.Fatal(err)
	}
	sthlm, _ := time.LoadLocation("Europe/Stockholm")
	return New(Deps{Catalog: c, Auth: fakeAuth{signedIn: true}, Store: newFakeStore(),
		Now:     func() time.Time { return time.Date(2026, 9, 27, 8, 0, 0, 0, sthlm) },
		BaseURL: "https://matlistan.example.lan", Log: zerolog.Nop(), Build: b})
}

func TestChipShowsTheSignedInPerson(t *testing.T) {
	_, body := get(t, newServer(t, i18n.SV, true, newFakeStore()), "/week")
	for _, want := range []string{`href="/settings#account"`,
		`aria-label="Inloggad som Anna, öppna inställningar"`, `>A</a>`} {
		if !strings.Contains(body, want) {
			t.Errorf("week page lacks %s", want)
		}
	}
}

// A family member linked to this login names the chip, not the Keycloak name.
func TestLinkedMemberNamesTheChip(t *testing.T) {
	st := newFakeStore()
	id, _ := st.CreateMember(t.Context(), household.Member{Name: "Birgitta", BirthYear: 1985})
	if err := st.LinkMember(t.Context(), id, "sub-anna"); err != nil {
		t.Fatal(err)
	}
	_, body := get(t, newServer(t, i18n.SV, true, st), "/recipes")
	if !strings.Contains(body, `aria-label="Inloggad som Birgitta, öppna inställningar"`) ||
		!strings.Contains(body, `>B</a>`) {
		t.Error("chip does not use the linked member's name")
	}
}

func TestSettingsShowsTheAccount(t *testing.T) {
	_, body := get(t, newServer(t, i18n.SV, true, newFakeStore()), "/settings")
	for _, want := range []string{`id="account"`, "Konto", "Inloggad som Anna",
		"anna@example.org", `<form method="post" action="/auth/logout"`, "Logga ut"} {
		if !strings.Contains(body, want) {
			t.Errorf("settings lacks %s", want)
		}
	}
}

func TestViewerWithoutSessionIsNil(t *testing.T) {
	s := buildServer(Deps{Catalog: mustCatalog(t, i18n.SV), Auth: fakeAuth{}, Store: newFakeStore(),
		Now: time.Now, Log: zerolog.Nop()})
	r, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	if v := s.viewer(r); v != nil {
		t.Fatalf("viewer = %+v, want nil", v)
	}
}

func TestFooterShowsTheBuild(t *testing.T) {
	b := release.Info{Version: "0.1.0", Commit: _testSHA,
		CommitTime: time.Date(2026, 9, 25, 12, 2, 0, 0, time.UTC),
		BuildTime:  time.Date(2026, 9, 25, 12, 10, 0, 0, time.UTC)}
	_, body := get(t, buildServerWith(t, i18n.SV, b), "/week")
	for _, want := range []string{"<footer", "v0.1.0",
		`href="https://github.com/aldersfors/matlistan/commit/` + _testSHA + `"`, ">abcdef0<",
		`incheckad <time datetime="2026-09-25T12:02:00Z">25 sep 14:02</time>`,
		`byggd <time datetime="2026-09-25T12:10:00Z">25 sep 14:10</time>`} {
		if !strings.Contains(body, want) {
			t.Errorf("footer lacks %s", want)
		}
	}
	_, en := get(t, buildServerWith(t, i18n.EN, b), "/week")
	if !strings.Contains(en, "committed <time") || !strings.Contains(en, "built <time") {
		t.Error("English footer is not localized")
	}
}

// A dev build from a dirty tree says so and links nowhere.
func TestFooterDevBuild(t *testing.T) {
	_, body := get(t, buildServerWith(t, i18n.SV, release.Info{Version: "dev", Commit: _testSHA,
		Dirty: true}), "/week")
	if !strings.Contains(body, ">dev<") || !strings.Contains(body, "abcdef0-dirty") {
		t.Error("dev build not shown")
	}
	if strings.Contains(body, "github.com/aldersfors/matlistan/commit") {
		t.Error("dirty build links to a commit")
	}
	if strings.Contains(body, "incheckad") || strings.Contains(body, "byggd") {
		t.Error("unknown times shown")
	}
}

func mustCatalog(t *testing.T, l i18n.Locale) *i18n.Catalog {
	t.Helper()
	c, err := i18n.Load(l)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Without a name the email is the name, and it is not shown a second time.
func TestViewerWithOnlyAnEmail(t *testing.T) {
	s := buildServer(Deps{Catalog: mustCatalog(t, i18n.SV), Auth: fakeAuth{}, Store: newFakeStore(),
		Now: time.Now, Log: zerolog.Nop()})
	r, _ := http.NewRequestWithContext(auth.WithSession(t.Context(),
		auth.Session{Subject: "sub-x", Email: "dev@localhost"}), http.MethodGet, "/", nil)
	v := s.viewer(r)
	if v == nil || v.Name != "dev@localhost" || v.Initial != "D" || v.Email != "" {
		t.Fatalf("viewer = %+v", v)
	}
}
