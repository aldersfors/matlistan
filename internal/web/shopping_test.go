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

// An earlier week's list stays reachable after the next week is approved.
func TestShoppingWeekNavigation(t *testing.T) {
	h, _ := approvedWeek(t)
	_, body := get(t, h, "/shopping")
	if !strings.Contains(body, `href="/shopping?y=2026&amp;w=39"`) ||
		!strings.Contains(body, `href="/shopping?y=2026&amp;w=41"`) {
		t.Fatalf("navigation: %.600s", body)
	}
	_, body = get(t, h, "/shopping?y=2026&w=39")
	if !strings.Contains(body, "Vecka 39 har ingen inköpslista.") ||
		!strings.Contains(body, `href="/shopping?y=2026&amp;w=38"`) ||
		!strings.Contains(body, `href="/shopping?y=2026&amp;w=40"`) {
		t.Fatalf("week without a list: %.600s", body)
	}
	if res, _ := get(t, h, "/shopping?y=2026&w=99"); res.StatusCode != http.StatusBadRequest {
		t.Errorf("bad week: %d", res.StatusCode)
	}
}

// Editing an older list returns to that list, not the current one.
func TestEditingAnOlderListStaysOnIt(t *testing.T) {
	h, st := approvedWeek(t)
	w39 := _w40.AddWeeks(-1)
	st.lists[w39] = shopping.List{ID: 900, Key: w39, Items: []shopping.Item{
		{ID: 901, Name: "mjölk", Section: "dairy"}}}
	week := url.Values{"y": {"2026"}, "w": {"39"}}
	back := "/shopping?y=2026&w=39"

	add := url.Values{"list": {"900"}, "name": {"tandkräm"}, "y": {"2026"}, "w": {"39"}}
	if rec := post(t, h, "/shopping/items", add); rec.Code != http.StatusSeeOther ||
		rec.Header().Get("Location") != back {
		t.Fatalf("add: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	add.Set("name", " ")
	rec := post(t, h, "/shopping/items", add)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "mjölk") ||
		strings.Contains(rec.Body.String(), "pumpa") {
		t.Fatalf("add blank shows the wrong list: %d", rec.Code)
	}
	if rec := post(t, h, "/fragments/shopping/items/901/toggle",
		url.Values{"checked": {"1"}, "y": {"2026"}, "w": {"39"}}); rec.Header().Get("Location") != back {
		t.Fatalf("toggle: %q", rec.Header().Get("Location"))
	}
	manual := st.lists[w39].Items[len(st.lists[w39].Items)-1]
	if rec := post(t, h, "/shopping/items/"+itoa(manual.ID)+"/delete", week); rec.Header().Get("Location") != back {
		t.Fatalf("delete: %q", rec.Header().Get("Location"))
	}
	_, body := get(t, h, back)
	if !strings.Contains(body, `name="w" value="39"`) {
		t.Fatal("forms do not carry the week")
	}
}

func TestApprovedWeekLinksToItsList(t *testing.T) {
	h, _ := approvedWeek(t)
	if _, body := get(t, h, "/week?y=2026&w=40"); !strings.Contains(body, `href="/shopping?y=2026&amp;w=40"`) {
		t.Fatal("approved week has no link to its list")
	}
	if _, body := get(t, h, "/week?y=2026&w=41"); strings.Contains(body, `href="/shopping?y=2026&amp;w=41"`) {
		t.Fatal("draft week links to a list")
	}
}
