package store

import (
	"context"
	"errors"
	"testing"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/recipes"
)

func gitMember(key, name string) household.Member {
	return household.Member{Key: key, Managed: "inline", Name: name, BirthYear: 1985}
}

func gitRecipe(key, title string) recipes.Recipe {
	r := soup()
	r.Key, r.Managed, r.Title, r.Lang, r.Source = key, "inline", title, i18n.SV, "manual"
	return r
}

func TestSyncAdoptsUpdatesAndArchivesMembers(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.CreateMember(ctx, household.Member{Name: "Anna", BirthYear: 1990})
	if err := s.LinkMember(ctx, id, "sub-anna"); err != nil {
		t.Fatal(err)
	}
	res, err := s.SyncHousehold(ctx, []household.Member{gitMember("anna", "Anna J"), gitMember("bo", "Bo")}, nil, nil)
	if err != nil || res.Members != 2 {
		t.Fatalf("sync: %+v, %v", res, err)
	}
	m, _ := s.GetMember(ctx, id)
	if m.Name != "Anna J" || m.BirthYear != 1985 || m.Managed != "inline" || m.Subject != "sub-anna" {
		t.Fatalf("adopted = %+v", m)
	}
	// Bo removed from git: archived, not deleted; declaring him again brings him back.
	if _, err := s.SyncHousehold(ctx, []household.Member{gitMember("anna", "Anna J")}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if ms, _ := s.ListMembers(ctx); len(ms) != 1 {
		t.Fatalf("members after removal = %+v", ms)
	}
	if _, err := s.SyncHousehold(ctx, []household.Member{gitMember("anna", "Anna J"), gitMember("bo", "Bo")}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if ms, _ := s.ListMembers(ctx); len(ms) != 2 {
		t.Fatalf("members after return = %+v", ms)
	}
}

func TestSyncRecipesByKey(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.CreateRecipe(ctx, soup())
	own, _ := s.GetRecipe(ctx, id)
	r := gitRecipe(own.Key, "Soppa från git")
	r.Ingredients = []recipes.Ingredient{{Name: "morot", Quantity: 3, Unit: "pcs", Section: "produce"}}
	if _, err := s.SyncHousehold(ctx, nil, []recipes.Recipe{r, gitRecipe("ny", "Ny rätt")}, nil); err != nil {
		t.Fatal(err)
	}
	got, _ := s.GetRecipe(ctx, id)
	if got.Title != "Soppa från git" || got.Managed != "inline" || len(got.Ingredients) != 1 ||
		got.Source != own.Source {
		t.Fatalf("adopted = %+v", got)
	}
	if _, err := s.SyncHousehold(ctx, nil, []recipes.Recipe{r}, nil); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.ListRecipes(ctx, i18n.SV, ""); len(list) != 1 {
		t.Fatalf("recipes after removal = %+v", list)
	}
}

// Review focus 5: pasted from an imported recipe with a new key, it takes that row over.
func TestSyncAdoptsARecipeByItsLink(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	imp := soup()
	imp.Source, imp.SourceURL = "imported", "https://www.ica.se/recept/soppa-1/"
	id, _ := s.CreateRecipe(ctx, imp)
	r := gitRecipe("min-soppa", "Min soppa")
	r.SourceURL = imp.SourceURL
	if _, err := s.SyncHousehold(ctx, nil, []recipes.Recipe{r}, nil); err != nil {
		t.Fatalf("sync: %v", err)
	}
	got, _ := s.GetRecipe(ctx, id)
	if got.Key != "min-soppa" || got.Managed != "inline" || got.Title != "Min soppa" {
		t.Fatalf("not adopted by link: %+v", got)
	}
}

func TestSyncLinks(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	imp := soup()
	imp.Source, imp.SourceURL = "imported", "https://a.example/r1/"
	id, _ := s.CreateRecipe(ctx, imp)
	res, err := s.SyncHousehold(ctx, nil, nil, []string{"https://a.example/r1/", "https://a.example/r2/"})
	if err != nil || res.Links != 1 || len(res.MissingLinks) != 1 || res.MissingLinks[0] != "https://a.example/r2/" {
		t.Fatalf("sync: %+v, %v", res, err)
	}
	if got, _ := s.GetRecipe(ctx, id); got.Managed != "url" {
		t.Fatalf("link not adopted: %+v", got)
	}
	// Removing the link archives its recipe but not a full recipe with the same sourceURL.
	full := gitRecipe("full", "Full")
	full.SourceURL = "https://a.example/other/"
	if _, err := s.SyncHousehold(ctx, nil, []recipes.Recipe{full}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetRecipe(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("link recipe not archived: %v", err)
	}
	if list, _ := s.ListRecipes(ctx, i18n.SV, ""); len(list) != 1 || list[0].Title != "Full" {
		t.Fatalf("recipes = %+v", list)
	}
	// Declaring the link again brings the archived row back rather than importing it again.
	res, _ = s.SyncHousehold(ctx, nil, []recipes.Recipe{full}, []string{"https://a.example/r1/"})
	if res.Links != 1 || len(res.MissingLinks) != 0 {
		t.Fatalf("archived link not brought back: %+v", res)
	}
}

func TestSyncIsAllOrNothing(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	bad := gitRecipe("bad", "Bad")
	bad.Servings = 0 // violates the servings CHECK, so the insert fails mid-sync
	if _, err := s.SyncHousehold(ctx, []household.Member{gitMember("anna", "Anna")}, []recipes.Recipe{bad}, nil); err == nil {
		t.Fatal("bad sync succeeded")
	}
	if ms, _ := s.ListMembers(ctx); len(ms) != 0 {
		t.Fatalf("half a sync applied: %+v", ms)
	}
}

// Review focus 1: turning household off unlocks what git owned.
func TestReleaseHouseholdUnlocksEverything(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	if _, err := s.SyncHousehold(ctx, []household.Member{gitMember("anna", "Anna")},
		[]recipes.Recipe{gitRecipe("soppa", "Soppa")}, nil); err != nil {
		t.Fatal(err)
	}
	n, err := s.ReleaseHousehold(ctx)
	if err != nil || n != 2 {
		t.Fatalf("release = %d, %v", n, err)
	}
	ms, _ := s.ListMembers(ctx)
	if ms[0].Managed != "" {
		t.Fatalf("member still managed: %+v", ms[0])
	}
}

func TestCreateAndAdoptLinkRecipes(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	r := soup()
	r.Source, r.SourceURL = "imported", "https://a.example/r/"
	id, err := s.CreateLinkRecipe(ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetRecipe(ctx, id); got.Managed != "url" || got.Key == "" {
		t.Fatalf("link recipe = %+v", got)
	}
	if _, err := s.CreateLinkRecipe(ctx, r); !errors.Is(err, ErrDuplicateSource) {
		t.Fatalf("duplicate: %v", err)
	}
	if err := s.AdoptLink(ctx, "https://a.example/none/"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("adopt missing: %v", err)
	}
}
