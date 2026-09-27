package store

import (
	"context"
	"errors"
	"testing"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/weekplan"
)

// ratedWeek approves week 40 with recipe id on Monday and returns two member ids.
func ratedWeek(t *testing.T, s *Store, id int64) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	a, _ := s.CreateMember(ctx, household.Member{Name: "Anna", BirthYear: 1985})
	b, _ := s.CreateMember(ctx, household.Member{Name: "Leo", BirthYear: 2014})
	if err := s.SavePicks(ctx, _w40, weekplan.DefaultContext(7),
		[]weekplan.Pick{{Day: 1, RecipeID: id, Servings: 4}}, true); err != nil {
		t.Fatal(err)
	}
	return a, b
}

// Review focus 1 and 2.
func TestSetRating(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.CreateRecipe(ctx, soup())
	a, b := ratedWeek(t, s, id)
	if err := s.SetRating(ctx, _w40, 1, a, 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rating a draft week: %v", err)
	}
	if err := s.ApprovePlan(ctx, _w40, "x", nil, 0); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []struct {
		day    int
		member int64
	}{{2, a}, {1, 9999}} {
		if err := s.SetRating(ctx, _w40, bad.day, bad.member, 5); !errors.Is(err, ErrNotFound) {
			t.Errorf("day %d member %d: %v", bad.day, bad.member, err)
		}
	}
	if err := s.SetRating(ctx, _w40, 1, a, 2); err == nil {
		t.Error("score 2 stored")
	}
	if err := s.SetRating(ctx, _w40, 1, a, 5); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRating(ctx, _w40, 1, b, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRating(ctx, _w40, 1, b, 3); err != nil { // changed their mind
		t.Fatal(err)
	}
	got, err := s.WeekRatings(ctx, _w40)
	if err != nil || got[1][a] != 5 || got[1][b] != 3 {
		t.Fatalf("ratings = %v, %v", got, err)
	}
	r, _ := s.GetRecipe(ctx, id)
	list, _ := s.ListRecipes(ctx, i18n.SV, "")
	if r.Rating.Count != 2 || r.Rating.Average != 4 || list[0].Rating.Average != 4 {
		t.Fatalf("rating = %+v, summary %+v", r.Rating, list[0].Rating)
	}
	if err := s.ArchiveMember(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRating(ctx, _w40, 1, b, 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived member: %v", err)
	}
}

// Review focus 3: a generated dish joins the library at average 4.
func TestWellRatedGeneratedRecipesJoinTheLibrary(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	gen := soup()
	gen.Title, gen.Source = "Pumpasoppa", "generated"
	id, _ := s.CreateRecipe(ctx, gen)
	a, b := ratedWeek(t, s, id)
	if err := s.ApprovePlan(ctx, _w40, "x", nil, 0); err != nil {
		t.Fatal(err)
	}
	_ = s.SetRating(ctx, _w40, 1, a, 5)
	_ = s.SetRating(ctx, _w40, 1, b, 1) // average 3: still hidden
	if list, _ := s.ListRecipes(ctx, i18n.SV, ""); len(list) != 0 {
		t.Fatalf("average 3 listed: %+v", list)
	}
	_ = s.SetRating(ctx, _w40, 1, b, 3) // average 4
	list, _ := s.ListRecipes(ctx, i18n.SV, "")
	cands, _ := s.ListCandidates(ctx, i18n.SV)
	if len(list) != 1 || len(cands) != 1 || cands[0].Rating.Count != 2 {
		t.Fatalf("library %+v candidates %+v", list, cands)
	}
}
