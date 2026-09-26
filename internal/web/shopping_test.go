package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/recipes"
	"github.com/jalet/matlistan/internal/shopping"
)

// approvedWeek plans and approves week 40 through the web, with a salt staple.
func approvedWeek(t *testing.T) (http.Handler, *fakeStore) {
	t.Helper()
	st := newFakeStore()
	pl := &fakePlanner{st: st}
	h, s := newPlanningServer(t, i18n.SV, st, pl)
	_ = st.AddStaple(t.Context(), "Salt")
	st.ingredients[_w40] = []shopping.Use{
		{Day: 1, Servings: 4, RecipeServings: 4, Ingredient: recipes.Ingredient{Name: "pumpa",
			Quantity: 1.5, Unit: "kg", Section: "produce"}},
		{Day: 2, Servings: 4, RecipeServings: 4, Ingredient: recipes.Ingredient{Name: "salt",
			Section: "pantry"}},
		{Day: 2, Servings: 4, RecipeServings: 4, Ingredient: recipes.Ingredient{Name: "vispgrädde",
			Quantity: 2, Unit: "dl", Section: "dairy"}},
	}
	post(t, h, "/week/generate", weekForm(nil))
	s.jobs.wait()
	if rec := post(t, h, "/week/approve", weekForm(nil)); rec.Code != http.StatusSeeOther {
		t.Fatalf("approve: %d", rec.Code)
	}
	return h, st
}

func TestApprovalCreatesTheList(t *testing.T) {
	h, st := approvedWeek(t)
	l, err := st.GetShoppingList(t.Context(), _w40)
	if err != nil || len(l.Items) != 2 || l.Excluded != 1 {
		t.Fatalf("list = %+v, %v", l, err)
	}
	_, body := get(t, h, "/shopping")
	for _, want := range []string{"Inköpslista", "Vecka 40", "Frukt och grönt", "1,5 kg pumpa",
		"2 dl vispgrädde", "2 varor kvar", "1 basvara utelämnad", "day-1", "day-2"} {
		if !strings.Contains(body, want) {
			t.Errorf("shopping page lacks %q", want)
		}
	}
	if strings.Contains(body, "salt") {
		t.Error("staple on the list")
	}
}

// Review focus 4: nothing approved yet.
func TestShoppingWithoutAList(t *testing.T) {
	_, body := get(t, newServer(t, i18n.SV, true, newFakeStore()), "/shopping")
	if !strings.Contains(body, "Ingen godkänd vecka än") {
		t.Fatal("empty state missing")
	}
}

// Review focus 5: toggling returns the row for htmx, twice; a removed item is 404.
func TestToggleAndManualItems(t *testing.T) {
	h, st := approvedWeek(t)
	l, _ := st.GetShoppingList(t.Context(), _w40)
	id := itoa(l.Items[0].ID)
	req := url.Values{}
	rec := postHX(t, h, "/fragments/shopping/items/"+id+"/toggle", req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "line-through") {
		t.Fatalf("toggle: %d %s", rec.Code, rec.Body.String())
	}
	rec = postHX(t, h, "/fragments/shopping/items/"+id+"/toggle", req)
	if strings.Contains(rec.Body.String(), "line-through") {
		t.Fatal("second toggle did not untick")
	}
	if rec := post(t, h, "/fragments/shopping/items/"+id+"/toggle", req); rec.Code != http.StatusSeeOther {
		t.Fatalf("toggle without htmx: %d", rec.Code)
	}
	if rec := post(t, h, "/shopping/items", url.Values{"list": {itoa(l.ID)}, "name": {"tandkräm"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("add: %d", rec.Code)
	}
	if rec := post(t, h, "/shopping/items", url.Values{"list": {itoa(l.ID)}, "name": {"  "}}); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("add blank: %d", rec.Code)
	}
	l, _ = st.GetShoppingList(t.Context(), _w40)
	manual := l.Items[len(l.Items)-1]
	if rec := post(t, h, "/shopping/items/"+itoa(manual.ID)+"/delete", nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := postHX(t, h, "/fragments/shopping/items/"+itoa(manual.ID)+"/toggle", req); rec.Code != http.StatusNotFound {
		t.Fatalf("toggle removed item: %d", rec.Code)
	}
	if rec := post(t, h, "/shopping/items/"+itoa(l.Items[0].ID)+"/delete", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("delete a planned item: %d", rec.Code)
	}
}

func TestNavHasShopping(t *testing.T) {
	_, body := get(t, newServer(t, i18n.SV, true, newFakeStore()), "/shopping")
	if !strings.Contains(body, `href="/shopping" aria-current="page"`) {
		t.Fatal("nav does not mark shopping")
	}
}
