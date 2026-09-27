package importer

import (
	"context"
	"errors"
	"testing"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/recipes"
)

type fakeFetcher struct {
	page Page
	err  error
}

func (f fakeFetcher) Fetch(context.Context, string) (Page, error) { return f.page, f.err }

type fakeNorm struct {
	translated, fromText bool
	err                  error
}

func (f *fakeNorm) Ingredients(_ context.Context, lines []string, _ i18n.Locale) ([]recipes.Ingredient, []string, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	out := make([]recipes.Ingredient, len(lines))
	for i, l := range lines {
		out[i] = recipes.Ingredient{Name: l, Section: "other"}
	}
	return out, nil, nil
}

func (f *fakeNorm) FromText(context.Context, string, i18n.Locale) (Draft, error) {
	f.fromText = true
	return Draft{Title: "Från text", Servings: 2, TotalMinutes: 20, Steps: []string{"Gör."},
		IngredientLines: []string{"1 ägg"}}, f.err
}

func (f *fakeNorm) Translate(_ context.Context, d Draft, l i18n.Locale) (Draft, error) {
	f.translated = true
	d.Lang = string(l)
	return d, f.err
}

func page(t *testing.T, name string) Page {
	return Page{URL: "https://www.ica.se/recept/kottbullar/", HTML: fixture(t, name)}
}

func TestImportWithModel(t *testing.T) {
	n := &fakeNorm{}
	r, notes, err := Importer{Fetch: fakeFetcher{page: page(t, "ica.html")}, Norm: n, Lang: i18n.SV}.
		Import(context.Background(), "https://www.ica.se/recept/kottbullar/")
	if err != nil || r.Title != "Köttbullar med potatismos" || r.Source != "imported" ||
		r.SourceURL != "https://www.ica.se/recept/kottbullar" || len(r.Ingredients) != 4 ||
		r.Servings != 4 || r.Lang != i18n.SV || n.translated || len(notes) != 0 {
		t.Fatalf("recipe = %+v notes %v err %v translated %v", r, notes, err, n.translated)
	}
}

func TestImportTranslatesOtherLanguages(t *testing.T) {
	n := &fakeNorm{}
	if _, notes, err := (Importer{Fetch: fakeFetcher{page: page(t, "koket.html")}, Norm: n, Lang: i18n.SV}).
		Import(context.Background(), "https://www.koket.se/x"); err != nil || !n.translated ||
		len(notes) != 1 || notes[0] != "import.note.translated" {
		t.Fatalf("translated %v notes %v err %v", n.translated, notes, err)
	}
}

func TestImportWithoutModel(t *testing.T) {
	r, notes, err := Importer{Fetch: fakeFetcher{page: page(t, "ica.html")}, Lang: i18n.SV}.
		Import(context.Background(), "https://www.ica.se/recept/kottbullar")
	if err != nil || len(r.Ingredients) != 4 || r.Ingredients[0].Name != "500 g blandfärs" ||
		r.Ingredients[0].Unit != "" || len(notes) != 1 || notes[0] != "import.note.no_model" {
		t.Fatalf("recipe %+v notes %v err %v", r.Ingredients, notes, err)
	}
	if _, _, err := (Importer{Fetch: fakeFetcher{page: page(t, "nojsonld.html")}, Lang: i18n.SV}).
		Import(context.Background(), "https://x.se/r"); !errors.Is(err, ErrNoRecipe) {
		t.Fatalf("no JSON-LD and no model: %v", err)
	}
}

func TestImportFallsBackToText(t *testing.T) {
	n := &fakeNorm{}
	r, _, err := Importer{Fetch: fakeFetcher{page: page(t, "nojsonld.html")}, Norm: n, Lang: i18n.SV}.
		Import(context.Background(), "https://x.se/r")
	if err != nil || !n.fromText || r.Title != "Från text" {
		t.Fatalf("recipe %+v err %v", r, err)
	}
}

// A model failure still returns what the page itself said, with a note.
func TestImportKeepsTheDraftWhenTheModelFails(t *testing.T) {
	n := &fakeNorm{err: ErrModelUnavailable}
	r, notes, err := Importer{Fetch: fakeFetcher{page: page(t, "ica.html")}, Norm: n, Lang: i18n.SV}.
		Import(context.Background(), "https://www.ica.se/recept/kottbullar")
	if err != nil || r.Title == "" || len(r.Ingredients) != 4 || len(notes) != 1 ||
		notes[0] != "import.note.no_model" {
		t.Fatalf("recipe %+v notes %v err %v", r, notes, err)
	}
}

func TestImportPassesFetchErrors(t *testing.T) {
	if _, _, err := (Importer{Fetch: fakeFetcher{err: ErrNotAllowed}, Lang: i18n.SV}).
		Import(context.Background(), "https://10.0.0.1/"); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("err = %v", err)
	}
}
