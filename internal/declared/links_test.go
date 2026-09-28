package declared

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/recipes/importer"
	"github.com/aldersfors/matlistan/internal/store"
)

type fakeImporter struct {
	mu    sync.Mutex
	calls map[string]int
	fail  map[string][]error // errors returned in order, then success
	final map[string]string  // the URL the page redirects to
}

func (f *fakeImporter) Import(_ context.Context, url string) (recipes.Recipe, []string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.calls == nil {
		f.calls = map[string]int{}
	}
	f.calls[url]++
	if errs := f.fail[url]; len(errs) > 0 {
		err := errs[0]
		f.fail[url] = errs[1:]
		return recipes.Recipe{}, nil, err
	}
	src := url
	if f.final[url] != "" {
		src = f.final[url]
	}
	return recipes.Recipe{Title: "Pannkaka", Servings: 4, TotalMinutes: 30, Steps: []string{"Vispa."},
		Lang: "sv", Source: "imported", SourceURL: src,
		Ingredients: []recipes.Ingredient{{Name: "mjöl", Quantity: 3, Unit: "dl", Section: "pantry"}}}, nil, nil
}

type fakeLinkStore struct {
	mu      sync.Mutex
	saved   []recipes.Recipe
	adopted []string
	dupe    bool
}

func (s *fakeLinkStore) CreateLinkRecipe(_ context.Context, r recipes.Recipe) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.dupe {
		return 0, store.ErrDuplicateSource
	}
	s.saved = append(s.saved, r)
	return int64(len(s.saved)), nil
}

func (s *fakeLinkStore) AdoptLink(_ context.Context, link string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.adopted = append(s.adopted, link)
	return nil
}

func run(t *testing.T, imp *fakeImporter, st *fakeLinkStore, links ...string) string {
	t.Helper()
	var log bytes.Buffer
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	Links{Importer: imp, Store: st, Log: zerolog.New(&log), Retry: 10 * time.Millisecond,
		Timeout: time.Second}.Run(ctx, links)
	return log.String()
}

func TestLinksImportOnce(t *testing.T) {
	imp, st := &fakeImporter{}, &fakeLinkStore{}
	run(t, imp, st, "https://a.example/r")
	if len(st.saved) != 1 || imp.calls["https://a.example/r"] != 1 {
		t.Fatalf("saved %d, calls %v", len(st.saved), imp.calls)
	}
}

// Review focus 2: a redirect must not change the recipe's identity.
func TestLinksKeepTheDeclaredLink(t *testing.T) {
	imp := &fakeImporter{final: map[string]string{"https://a.example/r": "https://a.example/new-r"}}
	st := &fakeLinkStore{}
	run(t, imp, st, "https://a.example/r")
	if len(st.saved) != 1 || st.saved[0].SourceURL != "https://a.example/r" {
		t.Fatalf("saved = %+v", st.saved)
	}
}

func TestLinksRetryTransientFailures(t *testing.T) {
	imp := &fakeImporter{fail: map[string][]error{"https://a.example/r": {importer.ErrUnreachable}}}
	st := &fakeLinkStore{}
	run(t, imp, st, "https://a.example/r")
	if len(st.saved) != 1 || imp.calls["https://a.example/r"] != 2 {
		t.Fatalf("saved %d, calls %v", len(st.saved), imp.calls)
	}
}

// Review focus 3: a page that can never import is not fetched again until the next start.
func TestLinksDropPermanentFailures(t *testing.T) {
	imp := &fakeImporter{fail: map[string][]error{
		"https://a.example/none": {importer.ErrNoRecipe, importer.ErrNoRecipe, importer.ErrNoRecipe}}}
	st := &fakeLinkStore{}
	log := run(t, imp, st, "https://a.example/none", "https://b.example/ok")
	if imp.calls["https://a.example/none"] != 1 || len(st.saved) != 1 {
		t.Fatalf("calls %v, saved %d", imp.calls, len(st.saved))
	}
	if strings.Contains(log, "/none") || !strings.Contains(log, "a.example") {
		t.Fatalf("log should name the host only:\n%s", log)
	}
}

func TestLinksDropInvalidRecipes(t *testing.T) {
	st := &fakeLinkStore{}
	bad := &badImporter{}
	log := runWith(t, bad, st, "https://a.example/r")
	if bad.calls != 1 || len(st.saved) != 0 || !strings.Contains(log, "total_minutes") {
		t.Fatalf("calls %d, saved %d, log:\n%s", bad.calls, len(st.saved), log)
	}
}

type badImporter struct{ calls int }

func (b *badImporter) Import(context.Context, string) (recipes.Recipe, []string, error) {
	b.calls++
	return recipes.Recipe{Title: "X", Servings: 4, Steps: []string{"x"}, Lang: "sv",
		Ingredients: []recipes.Ingredient{{Name: "x", Section: "other"}}}, nil, nil // no total time
}

func runWith(t *testing.T, imp LinkImporter, st *fakeLinkStore, links ...string) string {
	t.Helper()
	var log bytes.Buffer
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	Links{Importer: imp, Store: st, Log: zerolog.New(&log), Retry: 10 * time.Millisecond,
		Timeout: time.Second}.Run(ctx, links)
	return log.String()
}

func TestLinksAdoptAnAppImport(t *testing.T) {
	st := &fakeLinkStore{dupe: true}
	run(t, &fakeImporter{}, st, "https://a.example/r")
	if len(st.adopted) != 1 || st.adopted[0] != "https://a.example/r" {
		t.Fatalf("adopted = %v", st.adopted)
	}
}

func TestLinksStopWithTheServer(t *testing.T) {
	imp := &fakeImporter{fail: map[string][]error{"https://a.example/r": {
		importer.ErrUnreachable, importer.ErrUnreachable, importer.ErrUnreachable}}}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		Links{Importer: imp, Store: &fakeLinkStore{}, Log: zerolog.Nop(), Retry: time.Hour,
			Timeout: time.Second}.Run(ctx, []string{"https://a.example/r"})
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Run did not stop")
	}
}
