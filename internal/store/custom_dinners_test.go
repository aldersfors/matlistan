package store

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/weekplan"
)

// A dinner set by hand is a recipe or typed text. Either way it is locked, so planning again
// keeps it, and only a draft takes it.
func TestSetEntryByHand(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.CreateRecipe(ctx, soup())
	other, _ := s.CreateRecipe(ctx, soup())
	c := weekplan.DefaultContext(7)
	// A week without a row: the dinner creates the draft.
	if err := s.SetEntry(ctx, _w40, c, 2, 0, "Hämtmat", 4); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEntry(ctx, _w40, c, 3, id, "", 3); err != nil {
		t.Fatal(err)
	}
	p, err := s.GetPlan(ctx, _w40)
	if err != nil || p.Status != weekplan.StatusDraft || len(p.Entries) != 2 {
		t.Fatalf("plan = %+v, %v", p, err)
	}
	typed, rec := p.Entries[0], p.Entries[1]
	if typed.Day != 2 || !typed.Custom() || typed.Title != "Hämtmat" || typed.TotalMinutes != 0 ||
		!typed.Locked || rec.RecipeID != id || rec.Title != "Ärtsoppa" || rec.Servings != 3 ||
		rec.TotalMinutes != 40 || !rec.Locked {
		t.Fatalf("entries = %+v", p.Entries)
	}

	// Planning the whole week again keeps both and fills the other days.
	if err := s.SavePicks(ctx, _w40, c, []weekplan.Pick{{Day: 1, RecipeID: other, Servings: 4},
		{Day: 2, RecipeID: other, Servings: 4}}, true); err != nil {
		t.Fatal(err)
	}
	if p, _ = s.GetPlan(ctx, _w40); len(p.Entries) != 3 || p.Entries[1].Title != "Hämtmat" {
		t.Fatalf("after planning %+v", p.Entries)
	}

	// From text to a recipe and back.
	if err := s.SetEntry(ctx, _w40, c, 2, id, "", 4); err != nil {
		t.Fatal(err)
	}
	if p, _ = s.GetPlan(ctx, _w40); p.Entries[1].RecipeID != id {
		t.Fatalf("to recipe %+v", p.Entries[1])
	}
	if err := s.SetEntry(ctx, _w40, c, 2, 0, "Gäster", 4); err != nil {
		t.Fatal(err)
	}
	if p, _ = s.GetPlan(ctx, _w40); !p.Entries[1].Custom() || p.Entries[1].Title != "Gäster" {
		t.Fatalf("to text %+v", p.Entries[1])
	}

	// Unlocked by hand, a typed dinner is replaced by the next plan.
	if err := s.SetEntryLocked(ctx, _w40, 2, false); err != nil {
		t.Fatal(err)
	}
	if err := s.SavePicks(ctx, _w40, c, []weekplan.Pick{{Day: 2, RecipeID: other, Servings: 4}},
		false); err != nil {
		t.Fatal(err)
	}
	if p, _ = s.GetPlan(ctx, _w40); p.Entries[1].RecipeID != other || p.Entries[1].Custom() {
		t.Fatalf("replanned %+v", p.Entries[1])
	}

	if err := s.ClearEntry(ctx, _w40, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearEntry(ctx, _w40, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("clear twice: %v", err)
	}

	if err := s.ApprovePlan(ctx, _w40, "x", nil, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEntry(ctx, _w40, c, 4, 0, "Gäster", 4); !errors.Is(err, ErrApproved) {
		t.Fatalf("set on approved: %v", err)
	}
	if err := s.ClearEntry(ctx, _w40, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("clear on approved: %v", err)
	}
}

// Setting a dinner never overwrites conditions saved for the week.
func TestSetEntryKeepsConditions(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	c := weekplan.DefaultContext(7)
	c.Days[0].Guests = 3
	if err := s.SaveContext(ctx, _w40, c, [7]int{}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEntry(ctx, _w40, weekplan.DefaultContext(7), 1, 0, "Gäster", 6); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.GetPlan(ctx, _w40); p.Context.Days[0].Guests != 3 {
		t.Fatalf("conditions overwritten: %+v", p.Context.Days[0])
	}
}

// The database itself refuses a dinner that is both or neither.
func TestEntryIsRecipeOrText(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.CreateRecipe(ctx, soup())
	if err := s.SetEntry(ctx, _w40, weekplan.DefaultContext(7), 1, 0, "Gäster", 4); err != nil {
		t.Fatal(err)
	}
	p, _ := s.GetPlan(ctx, _w40)
	if _, err := s.pool.Exec(ctx, `INSERT INTO plan_entries (plan_id, day, recipe_id, title,
		servings) VALUES ($1, 5, $2, 'Gäster', 4)`, p.ID, id); err == nil {
		t.Error("both recipe and text stored")
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO plan_entries (plan_id, day, recipe_id, title,
		servings) VALUES ($1, 6, NULL, '', 4)`, p.ID); err == nil {
		t.Error("neither recipe nor text stored")
	}
}

// A typed dinner needs no shopping, is not history for the planner and cannot be rated.
func TestTypedDinnersStayOutOfListsAndRatings(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	if err := s.SetEntry(ctx, _w40, weekplan.DefaultContext(7), 1, 0, "Hämtmat", 4); err != nil {
		t.Fatal(err)
	}
	if uses, err := s.PlanIngredients(ctx, _w40); err != nil || len(uses) != 0 {
		t.Fatalf("ingredients = %+v, %v", uses, err)
	}
	if err := s.ApprovePlan(ctx, _w40, "x", nil, 0); err != nil {
		t.Fatalf("approve a typed-only week: %v", err)
	}
	if got, err := s.CookedSince(ctx, _w40, _w40.AddWeeks(1)); err != nil || len(got) != 0 {
		t.Fatalf("cooked = %+v, %v", got, err)
	}
	a, _ := s.CreateMember(ctx, household.Member{Name: "Anna", BirthYear: 1985})
	if err := s.SetRating(ctx, _w40, 1, a, 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("rate a typed dinner: %v", err)
	}
}

// Rolling back drops typed dinners, which the older schema cannot hold, and keeps the rest.
func TestCustomDinnersMigrationDown(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id, _ := s.CreateRecipe(ctx, soup())
	c := weekplan.DefaultContext(7)
	if err := s.SetEntry(ctx, _w40, c, 1, id, "", 4); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEntry(ctx, _w40, c, 2, 0, "Hämtmat", 4); err != nil {
		t.Fatal(err)
	}
	db := stdlib.OpenDB(*s.pool.Config().ConnConfig)
	defer func() { _ = db.Close() }()
	sub, _ := fs.Sub(_migrations, "migrations")
	p, err := goose.NewProvider(goose.DialectPostgres, db, sub)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DownTo(ctx, 10); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM plan_entries`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("entries after down = %d, %v", n, err)
	}
}
