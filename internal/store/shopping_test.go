package store

import (
	"context"
	"errors"
	"testing"

	"github.com/jalet/matlistan/internal/apitoken"
	"github.com/jalet/matlistan/internal/shopping"
	"github.com/jalet/matlistan/internal/weekplan"
)

func plannedWeek(t *testing.T, s *Store, k weekplan.Key) int64 {
	t.Helper()
	ctx := context.Background()
	id, err := s.CreateRecipe(ctx, soup())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SavePicks(ctx, k, weekplan.DefaultContext(7),
		[]weekplan.Pick{{Day: 4, RecipeID: id, Servings: 8}}, true); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestPlanIngredientsIncludeArchivedRecipes(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	id := plannedWeek(t, s, _w40)
	if err := s.ArchiveRecipe(ctx, id); err != nil {
		t.Fatal(err)
	}
	uses, err := s.PlanIngredients(ctx, _w40)
	if err != nil || len(uses) != 2 || uses[0].Day != 4 || uses[0].Servings != 8 ||
		uses[0].RecipeServings != 4 || uses[0].Ingredient.Name != "gul ärtsoppa" {
		t.Fatalf("uses = %+v, %v", uses, err)
	}
}

func TestApproveCreatesTheList(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	plannedWeek(t, s, _w40)
	items := []shopping.Item{{Name: "gul ärtsoppa", Section: "pantry", Quantity: 4, Unit: "pcs",
		Days: []int{4}}, {Name: "senap", Section: "pantry", Days: []int{4}, Optional: true}}
	if err := s.ApprovePlan(ctx, _w40, "sub-anna", items, 2); err != nil {
		t.Fatal(err)
	}
	l, err := s.GetShoppingList(ctx, _w40)
	if err != nil || len(l.Items) != 2 || l.Excluded != 2 || l.Items[0].Quantity != 4 ||
		l.Items[0].Days[0] != 4 || !l.Items[1].Optional || l.Key != _w40 {
		t.Fatalf("list = %+v, %v", l, err)
	}
	if err := s.ApprovePlan(ctx, _w40, "x", items, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second approval: %v", err)
	}
}

func TestCurrentShoppingList(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	if _, err := s.CurrentShoppingList(ctx, _w40); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no list yet: %v", err)
	}
	w39 := weekplan.Key{Year: 2026, Week: 39}
	for _, k := range []weekplan.Key{w39, _w40, {Year: 2026, Week: 41}} {
		plannedWeek(t, s, k)
		if err := s.ApprovePlan(ctx, k, "x", []shopping.Item{{Name: "a", Section: "other"}}, 0); err != nil {
			t.Fatal(err)
		}
	}
	if l, err := s.CurrentShoppingList(ctx, _w40); err != nil || l.Key != _w40 {
		t.Fatalf("current = %+v, %v", l.Key, err)
	}
}

// Review focus 5: toggling twice, and a removed item is not found.
func TestItemsToggleAddRemove(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	plannedWeek(t, s, _w40)
	if err := s.ApprovePlan(ctx, _w40, "x", []shopping.Item{{Name: "pumpa", Section: "produce",
		Quantity: 1, Unit: "kg"}}, 0); err != nil {
		t.Fatal(err)
	}
	l, _ := s.GetShoppingList(ctx, _w40)
	id := l.Items[0].ID
	if it, err := s.ToggleItem(ctx, id); err != nil || !it.Checked {
		t.Fatalf("toggle = %+v, %v", it, err)
	}
	if it, _ := s.ToggleItem(ctx, id); it.Checked {
		t.Fatal("second toggle")
	}
	if err := s.AddManualItem(ctx, l.ID, "tandkräm"); err != nil {
		t.Fatal(err)
	}
	l, _ = s.GetShoppingList(ctx, _w40)
	manual := l.Items[1]
	if !manual.Manual || manual.Name != "tandkräm" || manual.Section != "other" {
		t.Fatalf("manual = %+v", manual)
	}
	if err := s.RemoveManualItem(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("removing a planned item: %v", err)
	}
	if err := s.RemoveManualItem(ctx, manual.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ToggleItem(ctx, manual.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("toggle removed: %v", err)
	}
	if err := s.AddManualItem(ctx, 9999, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown list: %v", err)
	}
}

// Review focus 3: keys are found by hash only; revoked keys stop working.
func TestAPITokens(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	plain, hash, _ := apitoken.New()
	if err := s.CreateAPIToken(ctx, "sub-anna", "Annas iPhone", hash); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.UseAPIToken(ctx, apitoken.Hash(plain)); !ok || err != nil {
		t.Fatalf("use = %v, %v", ok, err)
	}
	list, _ := s.ListAPITokens(ctx)
	if len(list) != 1 || list[0].Name != "Annas iPhone" || list[0].LastUsedAt == nil {
		t.Fatalf("tokens = %+v", list)
	}
	if ok, _ := s.UseAPIToken(ctx, apitoken.Hash("mlt_wrong")); ok {
		t.Fatal("wrong key accepted")
	}
	if err := s.RevokeAPIToken(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.UseAPIToken(ctx, hash); ok {
		t.Fatal("revoked key accepted")
	}
}
