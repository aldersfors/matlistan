package store

import (
	"context"
	"errors"
	"testing"

	"github.com/aldersfors/matlistan/internal/apitoken"
	"github.com/aldersfors/matlistan/internal/shopping"
	"github.com/aldersfors/matlistan/internal/weekplan"
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
	// Two people ticking the same item both mean "bought": the state is set, not flipped.
	for range 2 {
		if it, err := s.SetItemChecked(ctx, id, true); err != nil || !it.Checked {
			t.Fatalf("check = %+v, %v", it, err)
		}
	}
	if it, _ := s.SetItemChecked(ctx, id, false); it.Checked {
		t.Fatal("uncheck")
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
	if _, err := s.SetItemChecked(ctx, manual.ID, true); !errors.Is(err, ErrNotFound) {
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

// Review focus: a rebuild replaces the planned lines, carries ticks by base name, keeps
// hand-added items with their ticks, and updates the staple count.
func TestRebuildShoppingList(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	if err := s.RebuildShoppingList(ctx, _w40, nil, 0); !errors.Is(err, ErrNotFound) {
		t.Fatalf("no list yet: %v", err)
	}
	plannedWeek(t, s, _w40)
	if err := s.ApprovePlan(ctx, _w40, "x", []shopping.Item{
		{Name: "morötter", Section: "produce", Quantity: 10, Unit: "pcs"},
		{Name: "salt", Section: "pantry"},
		{Name: "grädde", Section: "dairy", Quantity: 2, Unit: "dl"}}, 1); err != nil {
		t.Fatal(err)
	}
	l, _ := s.GetShoppingList(ctx, _w40)
	if err := s.AddManualItem(ctx, l.ID, "tandkräm"); err != nil {
		t.Fatal(err)
	}
	l, _ = s.GetShoppingList(ctx, _w40)
	for _, it := range l.Items {
		if it.Name == "morötter" || it.Name == "tandkräm" {
			if _, err := s.SetItemChecked(ctx, it.ID, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	built := []shopping.Item{
		{Name: "morot", Section: "produce", Quantity: 12, Unit: "pcs", Days: []int{1, 2}},
		{Name: "grädde", Section: "dairy", Quantity: 4, Unit: "dl"}}
	if err := s.RebuildShoppingList(ctx, _w40, built, 2); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetShoppingList(ctx, _w40)
	if err != nil || got.ID != l.ID || got.Excluded != 2 || len(got.Items) != 3 {
		t.Fatalf("list = %+v, %v", got, err)
	}
	want := []struct {
		name    string
		checked bool
		manual  bool
	}{{"morot", true, false}, {"grädde", false, false}, {"tandkräm", true, true}}
	for i, w := range want {
		it := got.Items[i]
		if it.Name != w.name || it.Checked != w.checked || it.Manual != w.manual {
			t.Errorf("item %d = %+v, want %+v", i, it, w)
		}
	}
	if got.Items[0].Quantity != 12 || len(got.Items[0].Days) != 2 {
		t.Errorf("morot = %+v", got.Items[0])
	}
}
