package store

import (
	"context"
	"errors"
	"testing"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/recipes"
)

func soup() recipes.Recipe {
	return recipes.Recipe{Title: "Ärtsoppa", Lang: i18n.SV, Servings: 4, ActiveMinutes: 10,
		TotalMinutes: 40, Source: "manual", Tags: []string{"torsdag"},
		Steps:     []string{"Värm soppan", "Servera med senap"},
		Allergens: []string{"mustard"},
		Ingredients: []recipes.Ingredient{
			{Name: "gul ärtsoppa", Quantity: 2, Unit: "pcs", Section: "pantry"},
			{Name: "senap", Section: "pantry", Optional: true},
		}}
}

func TestRecipeRoundTrip(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, err := s.CreateRecipe(ctx, soup())
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.GetRecipe(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	want := soup()
	want.ID = id
	if got.Title != want.Title || got.Lang != i18n.SV || len(got.Ingredients) != 2 ||
		got.Ingredients[0] != want.Ingredients[0] || got.Ingredients[1] != want.Ingredients[1] ||
		len(got.Steps) != 2 || got.Allergens[0] != "mustard" {
		t.Fatalf("got %+v", got)
	}
	got.Ingredients = got.Ingredients[:1]
	got.Title = "Ärtsoppa med fläsk"
	if err := s.UpdateRecipe(ctx, got); err != nil {
		t.Fatal(err)
	}
	again, _ := s.GetRecipe(ctx, id)
	if again.Title != "Ärtsoppa med fläsk" || len(again.Ingredients) != 1 {
		t.Fatalf("update: %+v", again)
	}
	if err := s.ArchiveRecipe(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRecipe(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived get: %v", err)
	}
	if err := s.UpdateRecipe(ctx, again); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived update: %v", err)
	}
}

// Review focus 1: Unicode case folding in search, and the list follows the deployment language.
func TestListRecipes(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	for _, r := range []recipes.Recipe{soup(),
		func() recipes.Recipe { r := soup(); r.Title = "Köttbullar"; return r }(),
		func() recipes.Recipe { r := soup(); r.Title = "Pea soup"; r.Lang = i18n.EN; return r }(),
	} {
		if _, err := s.CreateRecipe(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	for q, want := range map[string]int{"": 2, "ärt": 1, "ÄRT": 1, "bullar": 1, "pea": 0} {
		got, err := s.ListRecipes(ctx, i18n.SV, q)
		if err != nil || len(got) != want {
			t.Errorf("q=%q: %d results, want %d (%v)", q, len(got), want, err)
		}
	}
	all, _ := s.ListRecipes(ctx, i18n.SV, "")
	if all[0].Title != "Köttbullar" || all[0].TotalMinutes != 40 || all[0].Tags[0] != "torsdag" {
		t.Fatalf("summary = %+v", all[0])
	}
}

// Generated recipes stay out of the library and the candidates until ratings keep them (M5).
func TestGeneratedRecipesAreNotInTheLibrary(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	gen := soup()
	gen.Title, gen.Source = "Pumpasoppa", "generated"
	id, _ := s.CreateRecipe(ctx, gen)
	if _, err := s.CreateRecipe(ctx, soup()); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListRecipes(ctx, i18n.SV, "")
	cands, _ := s.ListCandidates(ctx, i18n.SV)
	if len(list) != 1 || list[0].Title != "Ärtsoppa" || len(cands) != 1 || cands[0].Title != "Ärtsoppa" {
		t.Fatalf("library %+v, candidates %+v", list, cands)
	}
	if r, err := s.GetRecipe(ctx, id); err != nil || r.Title != "Pumpasoppa" {
		t.Fatalf("generated recipe page: %+v, %v", r, err)
	}
}

func TestSourceURLRoundTripAndLookup(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	r := soup()
	r.Source, r.SourceURL = "imported", "https://www.ica.se/recept/artsoppa-1"
	id, err := s.CreateRecipe(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetRecipe(ctx, id); got.SourceURL != r.SourceURL || got.Source != "imported" {
		t.Fatalf("got %q %q", got.SourceURL, got.Source)
	}
	if found, err := s.FindRecipeBySourceURL(ctx, r.SourceURL); err != nil || found != id {
		t.Fatalf("find = %d, %v", found, err)
	}
	if _, err := s.FindRecipeBySourceURL(ctx, "https://www.ica.se/other"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing url: %v", err)
	}
	if _, err := s.CreateRecipe(ctx, r); err == nil {
		t.Fatal("second recipe with the same source URL stored")
	}
	manual := soup()
	manual.Title = "Ärtsoppa 2"
	if _, err := s.CreateRecipe(ctx, manual); err != nil {
		t.Fatalf("manual recipes without a URL must not collide: %v", err)
	}
	if _, err := s.CreateRecipe(ctx, manual); err != nil {
		t.Fatalf("two manual recipes: %v", err)
	}
}

// Review: an archived import must not block importing the same page again.
func TestReimportAfterArchive(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	r := soup()
	r.Source, r.SourceURL = "imported", "https://www.ica.se/recept/artsoppa-2"
	id, err := s.CreateRecipe(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ArchiveRecipe(ctx, id); err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateRecipe(ctx, r)
	if err != nil {
		t.Fatalf("re-import after archive: %v", err)
	}
	if found, _ := s.FindRecipeBySourceURL(ctx, r.SourceURL); found != again {
		t.Fatalf("find = %d, want %d", found, again)
	}
	if _, err := s.CreateRecipe(ctx, r); !errors.Is(err, ErrDuplicateSource) {
		t.Fatalf("live duplicate: %v, want ErrDuplicateSource", err)
	}
}
