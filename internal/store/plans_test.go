package store

import (
	"context"
	"errors"
	"testing"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/weekplan"
)

var _w40 = weekplan.Key{Year: 2026, Week: 40}

func TestPlanLifecycle(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	if _, err := s.GetPlan(ctx, _w40); !errors.Is(err, ErrNotFound) {
		t.Fatalf("empty week: %v", err)
	}
	c := weekplan.DefaultContext(7)
	c.Days[2].Busy = true
	if err := s.SaveContext(ctx, _w40, c, [7]int{}); err != nil {
		t.Fatal(err)
	}
	libID, _ := s.CreateRecipe(ctx, soup())
	gen := soup()
	gen.Title, gen.Source = "Pumpasoppa", "generated"
	picks := []weekplan.Pick{
		{Day: 1, New: &gen, Servings: 4, Why: "Pumpan är i säsong."},
		{Day: 4, RecipeID: libID, Servings: 4, Why: "Torsdag som vanligt."},
	}
	if err := s.SavePicks(ctx, _w40, c, picks, true); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetPlan(ctx, _w40)
	if err != nil || p.Status != weekplan.StatusDraft || !p.Context.Days[2].Busy ||
		len(p.Entries) != 2 || p.Entries[0].Title != "Pumpasoppa" || p.Entries[1].Day != 4 ||
		p.Entries[0].TotalMinutes != 40 || p.Error != "" {
		t.Fatalf("plan = %+v, %v", p, err)
	}
	newID := p.Entries[0].RecipeID
	if r, _ := s.GetRecipe(ctx, newID); r.Source != "generated" {
		t.Fatalf("generated recipe source = %q", r.Source)
	}

	// Swap Thursday only.
	if err := s.SavePicks(ctx, _w40, c, []weekplan.Pick{{Day: 4, RecipeID: newID,
		Servings: 3, Why: "Byte."}}, false); err != nil {
		t.Fatal(err)
	}
	p, _ = s.GetPlan(ctx, _w40)
	if len(p.Entries) != 2 || p.Entries[1].RecipeID != newID || p.Entries[1].Servings != 3 {
		t.Fatalf("after swap %+v", p.Entries)
	}

	if err := s.SetPlanError(ctx, _w40, c, "plan.error.invalid"); err != nil {
		t.Fatal(err)
	}
	if p, _ = s.GetPlan(ctx, _w40); p.Error != "plan.error.invalid" || len(p.Entries) != 2 {
		t.Fatalf("error kept entries? %+v", p)
	}

	if err := s.ApprovePlan(ctx, _w40, "sub-anna", nil, 0); err != nil {
		t.Fatal(err)
	}
	if p, _ = s.GetPlan(ctx, _w40); p.Status != weekplan.StatusApproved || p.ApprovedBy != "sub-anna" {
		t.Fatalf("approved %+v", p)
	}
	if err := s.SaveContext(ctx, _w40, c, [7]int{}); !errors.Is(err, ErrApproved) {
		t.Fatalf("context on approved: %v", err)
	}
	if err := s.SavePicks(ctx, _w40, c, picks, true); !errors.Is(err, ErrApproved) {
		t.Fatalf("picks on approved: %v", err)
	}
	if err := s.SetPlanError(ctx, _w40, c, "x"); !errors.Is(err, ErrApproved) {
		t.Fatalf("error on approved: %v", err)
	}
	if err := s.ApprovePlan(ctx, _w40, "x", nil, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("approve twice: %v", err)
	}
}

func TestApproveNeedsEntries(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	if err := s.SaveContext(ctx, _w40, weekplan.DefaultContext(7), [7]int{}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApprovePlan(ctx, _w40, "x", nil, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("approve empty: %v", err)
	}
}

// Review focus 5: history windows across the turn of the year; only approved weeks count.
func TestCookedSinceAndCandidates(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.CreateRecipe(ctx, soup())
	c := weekplan.DefaultContext(7)
	for _, k := range []weekplan.Key{{Year: 2026, Week: 52}, {Year: 2026, Week: 53},
		{Year: 2027, Week: 1}} {
		if err := s.SavePicks(ctx, k, c, []weekplan.Pick{{Day: 1, RecipeID: id, Servings: 4}},
			true); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range []weekplan.Key{{Year: 2026, Week: 52}, {Year: 2026, Week: 53}} {
		if err := s.ApprovePlan(ctx, k, "x", nil, 0); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.CookedSince(ctx, weekplan.Key{Year: 2026, Week: 53},
		weekplan.Key{Year: 2027, Week: 2})
	if err != nil || len(got) != 1 || got[0].Key != (weekplan.Key{Year: 2026, Week: 53}) ||
		got[0].Title != "Ärtsoppa" {
		t.Fatalf("cooked = %+v, %v", got, err)
	}
	cands, err := s.ListCandidates(ctx, i18n.SV)
	if err != nil || len(cands) != 1 || cands[0].ID != id || cands[0].Allergens[0] != "mustard" ||
		cands[0].TotalMinutes != 40 || len(cands[0].Ingredients) != 0 {
		t.Fatalf("candidates = %+v, %v", cands, err)
	}
}

// Conditions saved while a plan was running are not undone when the plan is stored.
func TestPlanningKeepsNewerConditions(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.CreateRecipe(ctx, soup())
	old := weekplan.DefaultContext(7)
	newer := weekplan.DefaultContext(7)
	newer.Days[2].Guests = 3
	if err := s.SaveContext(ctx, _w40, newer, [7]int{}); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePicks(ctx, _w40, old, []weekplan.Pick{{Day: 1, RecipeID: id, Servings: 4}},
		true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPlanError(ctx, _w40, old, "plan.error.invalid"); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.GetPlan(ctx, _w40); p.Context.Days[2].Guests != 3 {
		t.Fatalf("conditions overwritten: %+v", p.Context.Days[2])
	}
}

// A day marked "no dinner" loses its planned dinner, so it is neither approved nor cooked.
func TestSkippingADayDropsItsDinner(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.CreateRecipe(ctx, soup())
	c := weekplan.DefaultContext(7)
	if err := s.SavePicks(ctx, _w40, c, []weekplan.Pick{{Day: 1, RecipeID: id, Servings: 4},
		{Day: 4, RecipeID: id, Servings: 4}}, true); err != nil {
		t.Fatal(err)
	}
	c.Days[3].Skip = true
	if err := s.SaveContext(ctx, _w40, c, [7]int{}); err != nil {
		t.Fatal(err)
	}
	p, _ := s.GetPlan(ctx, _w40)
	if len(p.Entries) != 1 || p.Entries[0].Day != 1 {
		t.Fatalf("entries = %+v", p.Entries)
	}
}

// New conditions change a planned dinner's servings in the same save, so the shopping list
// and the recipe scale follow guests and away days; a zero keeps the day as it is.
func TestConditionsUpdateServings(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.CreateRecipe(ctx, soup())
	c := weekplan.DefaultContext(7)
	if err := s.SavePicks(ctx, _w40, c, []weekplan.Pick{{Day: 1, RecipeID: id, Servings: 4},
		{Day: 2, RecipeID: id, Servings: 4}}, true); err != nil {
		t.Fatal(err)
	}
	c.Days[0].Guests = 2
	if err := s.SaveContext(ctx, _w40, c, [7]int{6}); err != nil {
		t.Fatal(err)
	}
	p, _ := s.GetPlan(ctx, _w40)
	if len(p.Entries) != 2 || p.Entries[0].Servings != 6 || p.Entries[1].Servings != 4 {
		t.Fatalf("entries = %+v", p.Entries)
	}
}

// A locked dinner survives planning the week again, whole or in part.
func TestLockedDinnersSurvivePlanning(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	a, _ := s.CreateRecipe(ctx, soup())
	b, _ := s.CreateRecipe(ctx, soup())
	c := weekplan.DefaultContext(7)
	if err := s.SavePicks(ctx, _w40, c, []weekplan.Pick{{Day: 1, RecipeID: a, Servings: 4},
		{Day: 2, RecipeID: a, Servings: 4}}, true); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEntryLocked(ctx, _w40, 1, true); err != nil {
		t.Fatal(err)
	}
	// A whole-week plan and a stray pick for the locked day leave it as it was.
	if err := s.SavePicks(ctx, _w40, c, []weekplan.Pick{{Day: 1, RecipeID: b, Servings: 2},
		{Day: 2, RecipeID: b, Servings: 4}}, true); err != nil {
		t.Fatal(err)
	}
	p, _ := s.GetPlan(ctx, _w40)
	if len(p.Entries) != 2 || p.Entries[0].RecipeID != a || !p.Entries[0].Locked ||
		p.Entries[1].RecipeID != b || p.Entries[1].Locked {
		t.Fatalf("entries = %+v", p.Entries)
	}
	if err := s.SetEntryLocked(ctx, _w40, 1, false); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.GetPlan(ctx, _w40); p.Entries[0].Locked {
		t.Fatal("still locked")
	}
	if err := s.SetEntryLocked(ctx, _w40, 5, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lock a day without dinner: %v", err)
	}
}

func TestLockingNeedsADraft(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	plannedWeek(t, s, _w40)
	if err := s.ApprovePlan(ctx, _w40, "x", nil, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEntryLocked(ctx, _w40, 4, true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("lock in an approved week: %v", err)
	}
}
