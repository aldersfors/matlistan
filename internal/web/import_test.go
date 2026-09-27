package web

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/recipes/importer"
)

type fakeImporter struct {
	recipe recipes.Recipe
	notes  []string
	err    error
	calls  int
}

func (f *fakeImporter) Import(context.Context, string) (recipes.Recipe, []string, error) {
	f.calls++
	return f.recipe, f.notes, f.err
}

func importServer(t *testing.T, st *fakeStore, im RecipeImporter) http.Handler {
	t.Helper()
	sthlm, _ := time.LoadLocation("Europe/Stockholm")
	return New(Deps{Catalog: mustCatalog(t, i18n.SV), Auth: fakeAuth{signedIn: true}, Store: st,
		Now:     func() time.Time { return time.Date(2026, 9, 27, 8, 0, 0, 0, sthlm) },
		BaseURL: "https://matlistan.example.lan", Log: zerolog.Nop(), Importer: im})
}

func imported() recipes.Recipe {
	return recipes.Recipe{Title: "Köttbullar", Lang: i18n.SV, Servings: 4, TotalMinutes: 45,
		ActiveMinutes: 20, Steps: []string{"Blanda.", "Stek."}, Source: "imported",
		SourceURL:   "https://www.ica.se/recept/kottbullar",
		Ingredients: []recipes.Ingredient{{Name: "blandfärs", Quantity: 500, Unit: "g", Section: "meat_fish"}}}
}

func TestImportShowsAPrefilledForm(t *testing.T) {
	im := &fakeImporter{recipe: imported(), notes: []string{"import.note.translated"}}
	rec := post(t, importServer(t, newFakeStore(), im), "/recipes/import",
		url.Values{"url": {"https://www.ica.se/recept/kottbullar/"}})
	body := rec.Body.String()
	for _, want := range []string{`value="Köttbullar"`, `value="blandfärs"`, `value="500"`,
		`name="source_url" value="https://www.ica.se/recept/kottbullar"`, "Blanda.",
		"översatt"} {
		if !strings.Contains(body, want) {
			t.Errorf("form lacks %s", want)
		}
	}
}

// Review focus 3.
func TestImportDetectsDuplicates(t *testing.T) {
	st := newFakeStore()
	r := imported()
	id, _ := st.CreateRecipe(context.Background(), r)
	im := &fakeImporter{recipe: r}
	rec := post(t, importServer(t, st, im), "/recipes/import",
		url.Values{"url": {"https://WWW.ICA.SE/recept/kottbullar/#steg"}})
	if im.calls != 0 || !strings.Contains(rec.Body.String(), "Du har redan det här receptet") ||
		!strings.Contains(rec.Body.String(), `href="/recipes/`+itoa(id)+`"`) {
		t.Fatalf("calls %d body %.300s", im.calls, rec.Body.String())
	}
}

func TestImportErrors(t *testing.T) {
	for err, want := range map[error]string{
		importer.ErrNotAllowed:  "adressen får inte hämtas",
		importer.ErrUnreachable: "gick inte att hämta",
		importer.ErrNoRecipe:    "hittade inget recept",
		importer.ErrNotHTTPS:    "https",
	} {
		rec := post(t, importServer(t, newFakeStore(), &fakeImporter{err: err}), "/recipes/import",
			url.Values{"url": {"https://x.se/r"}})
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%v: %d %.200s", err, rec.Code, rec.Body.String())
		}
	}
	rec := post(t, importServer(t, newFakeStore(), &fakeImporter{}), "/recipes/import",
		url.Values{"url": {"inte en länk"}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("invalid url: %d", rec.Code)
	}
}

// Review focus 5.
func TestSaveImportedRecipe(t *testing.T) {
	st := newFakeStore()
	h := importServer(t, st, &fakeImporter{})
	form := meatballs()
	form.Set("source_url", "https://www.ica.se/recept/kottbullar")
	if rec := post(t, h, "/recipes", form); rec.Code != http.StatusSeeOther {
		t.Fatalf("save: %d %s", rec.Code, rec.Body.String())
	}
	manual := meatballs()
	manual.Set("title", "Egna köttbullar")
	if rec := post(t, h, "/recipes", manual); rec.Code != http.StatusSeeOther {
		t.Fatalf("manual save: %d", rec.Code)
	}
	var sources []string
	for _, r := range st.recipes {
		sources = append(sources, r.Source+" "+r.SourceURL)
	}
	joined := strings.Join(sources, "|")
	if !strings.Contains(joined, "imported https://www.ica.se/recept/kottbullar") ||
		!strings.Contains(joined, "manual ") {
		t.Fatalf("sources = %v", sources)
	}
}

func TestRecipePageShowsTheSource(t *testing.T) {
	st := newFakeStore()
	id, _ := st.CreateRecipe(context.Background(), imported())
	_, body := get(t, importServer(t, st, &fakeImporter{}), "/recipes/"+itoa(id))
	if !strings.Contains(body, `href="https://www.ica.se/recept/kottbullar"`) ||
		!strings.Contains(body, "Källa: www.ica.se") || !strings.Contains(body, `rel="noopener noreferrer"`) {
		t.Fatalf("recipe page lacks the source: %.400s", body)
	}
}

func TestImportFormOnTheLibrary(t *testing.T) {
	_, body := get(t, importServer(t, newFakeStore(), &fakeImporter{}), "/recipes")
	if !strings.Contains(body, `action="/recipes/import"`) || !strings.Contains(body, "Importera från länk") {
		t.Fatal("library lacks the import form")
	}
}

// A link that redirects to a recipe imported before still points at that recipe.
func TestImportDetectsDuplicatesAfterRedirect(t *testing.T) {
	st := newFakeStore()
	id, _ := st.CreateRecipe(context.Background(), imported())
	im := &fakeImporter{recipe: imported()} // the fetch ended at the stored URL
	rec := post(t, importServer(t, st, im), "/recipes/import",
		url.Values{"url": {"https://ica.se/r/12345"}})
	if !strings.Contains(rec.Body.String(), `href="/recipes/`+itoa(id)+`"`) {
		t.Fatalf("body %.300s", rec.Body.String())
	}
}

func TestCrossSiteImportIsRefused(t *testing.T) {
	im := &fakeImporter{}
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/recipes/import",
		strings.NewReader("url=https://x.se/r"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	importServer(t, newFakeStore(), im).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden || im.calls != 0 {
		t.Fatalf("status %d calls %d", rec.Code, im.calls)
	}
}

// Security review: a failed import logs the host and the reason, never the pasted URL's path
// or query, which can carry tokens.
func TestImportFailureLogsOnlyTheHost(t *testing.T) {
	var buf strings.Builder
	sthlm, _ := time.LoadLocation("Europe/Stockholm")
	err := fmt.Errorf("%w: Get \"https://www.ica.se/recept/x?token=hemligt\": dial tcp: timeout",
		importer.ErrUnreachable)
	h := New(Deps{Catalog: mustCatalog(t, i18n.SV), Auth: fakeAuth{signedIn: true},
		Store: newFakeStore(), Now: func() time.Time { return time.Date(2026, 9, 27, 8, 0, 0, 0, sthlm) },
		BaseURL: "https://matlistan.example.lan", Log: zerolog.New(&buf),
		Importer: &fakeImporter{err: err}})
	post(t, h, "/recipes/import", url.Values{"url": {"https://www.ica.se/recept/x?token=hemligt"}})
	log := buf.String()
	if strings.Contains(log, "hemligt") || strings.Contains(log, "/recept/x") {
		t.Errorf("log leaks the URL: %s", log)
	}
	if !strings.Contains(log, `"host":"www.ica.se"`) || !strings.Contains(log, "import.error.unreachable") {
		t.Errorf("log lacks host or reason: %s", log)
	}
}

// Review: a double tap on Save (or two tabs) lands on the recipe, not an error page.
func TestSavingAnImportTwiceOpensTheFirst(t *testing.T) {
	st := newFakeStore()
	h := importServer(t, st, &fakeImporter{})
	form := meatballs()
	form.Set("source_url", "https://www.ica.se/recept/kottbullar")
	first := post(t, h, "/recipes", form)
	second := post(t, h, "/recipes", form)
	if first.Code != http.StatusSeeOther || second.Code != http.StatusSeeOther ||
		first.Header().Get("Location") != second.Header().Get("Location") {
		t.Fatalf("first %d %q, second %d %q", first.Code, first.Header().Get("Location"),
			second.Code, second.Header().Get("Location"))
	}
	if len(st.recipes) != 1 {
		t.Fatalf("%d recipes stored", len(st.recipes))
	}
}
