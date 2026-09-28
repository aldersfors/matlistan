package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/shopping"
)

// approvedWeek plans and approves week 40 through the web, with a salt staple.
func approvedWeek(t *testing.T) (http.Handler, *fakeStore) {
	t.Helper()
	h, st, _ := approvedWeekServer(t)
	return h, st
}

// ratedWeek is approvedWeek with the clock moved past week 40, when every dinner is eaten.
func ratedWeek(t *testing.T) (http.Handler, *fakeStore) {
	t.Helper()
	h, st, s := approvedWeekServer(t)
	loc := s.Now().Location()
	s.Now = func() time.Time { return time.Date(2026, 10, 5, 18, 0, 0, 0, loc) }
	return h, st
}

func approvedWeekServer(t *testing.T) (http.Handler, *fakeStore, *server) {
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
	return h, st, s
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
	req := url.Values{"checked": {"1"}}
	rec := postHX(t, h, "/fragments/shopping/items/"+id+"/toggle", req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "line-through") {
		t.Fatalf("toggle: %d %s", rec.Code, rec.Body.String())
	}
	rec = postHX(t, h, "/fragments/shopping/items/"+id+"/toggle", url.Values{"checked": {"0"}})
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

// Two people ticking the same item leave it ticked; the button is named by what it shows.
func TestTickingSetsTheState(t *testing.T) {
	h, st := approvedWeek(t)
	l, _ := st.GetShoppingList(t.Context(), _w40)
	id := itoa(l.Items[0].ID)
	for range 2 {
		postHX(t, h, "/fragments/shopping/items/"+id+"/toggle", url.Values{"checked": {"1"}})
	}
	if l, _ := st.GetShoppingList(t.Context(), _w40); !l.Items[0].Checked {
		t.Fatal("second tick unticked the item")
	}
	_, body := get(t, h, "/shopping")
	if strings.Contains(body, "Markera pumpa som köpt") {
		t.Error("aria-label hides the amount from screen readers")
	}
	if !strings.Contains(body, `name="checked" value="0"`) {
		t.Error("a ticked row does not offer to untick")
	}
}

// Review focus: "Uppdatera listan" rebuilds from the approved week and the staples as they
// are now, keeps hand-added items and carries ticks.
func TestRebuildTheList(t *testing.T) {
	h, st := approvedWeek(t)
	_, body := get(t, h, "/shopping")
	if !strings.Contains(body, `action="/shopping/rebuild"`) || !strings.Contains(body, "Uppdatera listan") {
		t.Fatalf("no rebuild button: %.600s", body)
	}
	l, _ := st.GetShoppingList(t.Context(), _w40)
	if err := st.AddManualItem(t.Context(), l.ID, "tandkräm"); err != nil {
		t.Fatal(err)
	}
	for _, it := range l.Items {
		if it.Name == "pumpa" {
			_, _ = st.SetItemChecked(t.Context(), it.ID, true)
		}
	}
	_ = st.AddStaple(t.Context(), "Vispgrädde")
	rec := post(t, h, "/shopping/rebuild", weekForm(nil))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/shopping?y=2026&w=40&updated=1" {
		t.Fatalf("rebuild: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	got, _ := st.GetShoppingList(t.Context(), _w40)
	var names []string
	for _, it := range got.Items {
		names = append(names, it.Name)
		if it.Name == "pumpa" && !it.Checked {
			t.Error("pumpa lost its tick")
		}
	}
	if strings.Join(names, ",") != "pumpa,tandkräm" || got.Excluded != 2 {
		t.Fatalf("items = %v, excluded %d", names, got.Excluded)
	}
	if _, body := get(t, h, "/shopping?y=2026&w=40&updated=1"); !strings.Contains(body, "Listan är uppdaterad.") {
		t.Fatal("no confirmation")
	}
	if rec := post(t, h, "/shopping/rebuild", url.Values{"y": {"2026"}, "w": {"41"}}); rec.Code != http.StatusNotFound {
		t.Fatalf("week without a list: %d", rec.Code)
	}
}

func TestNoRebuildButtonWithoutAList(t *testing.T) {
	_, body := get(t, newServer(t, i18n.SV, true, newFakeStore()), "/shopping")
	if strings.Contains(body, "/shopping/rebuild") {
		t.Fatal("rebuild button without a list")
	}
}

func TestCrossSiteRebuildIsRefused(t *testing.T) {
	h, _ := approvedWeek(t)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/shopping/rebuild",
		strings.NewReader(weekForm(nil).Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", rec.Code)
	}
}
