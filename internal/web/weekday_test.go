package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/recipes"
)

func dayForm(day string, extra url.Values) url.Values {
	v := weekForm(url.Values{"day": {day}})
	for k, vs := range extra {
		v[k] = vs
	}
	return v
}

// Without a planner, a day gets a typed dinner or a recipe by hand, and loses it again.
func TestSetDinnerByHand(t *testing.T) {
	st := newFakeStore()
	id, _ := st.CreateRecipe(t.Context(), recipes.Recipe{Title: "Kycklinggryta", Lang: i18n.SV,
		Servings: 4, TotalMinutes: 35, Source: "manual"})
	h := newServer(t, i18n.SV, true, st)

	res, body := get(t, h, "/week/day?y=2026&w=40&day=2")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("day page: %d", res.StatusCode)
	}
	for _, want := range []string{"Middag Tis 29", "Skriv själv", "Kycklinggryta", "35 min"} {
		if !strings.Contains(body, want) {
			t.Errorf("day page lacks %q", want)
		}
	}
	if strings.Contains(body, "Ta bort middagen") {
		t.Error("remove offered for an empty day")
	}
	if _, body := get(t, h, "/week/day?y=2026&w=40&day=2&q=gryta"); !strings.Contains(body, "Kycklinggryta") {
		t.Error("search misses the recipe")
	}
	// Review focus 5: wildcards are plain text.
	for _, q := range []string{"%25", "_", "xyz"} {
		if _, body := get(t, h, "/week/day?y=2026&w=40&day=2&q="+q); strings.Contains(body, "Kycklinggryta") {
			t.Errorf("q=%s matched", q)
		}
	}

	rec := post(t, h, "/week/day", dayForm("2", url.Values{"title": {"  Falukorv   i ugn "}}))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/week?y=2026&w=40" {
		t.Fatalf("set text: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	e, ok := st.plans[_w40].Entry(2)
	if !ok || !e.Custom() || e.Title != "Falukorv i ugn" || !e.Locked || e.Servings < 1 {
		t.Fatalf("typed entry = %+v", e)
	}
	if _, body := get(t, h, "/week/day?y=2026&w=40&day=2"); !strings.Contains(body, `value="Falukorv i ugn"`) ||
		!strings.Contains(body, "Ta bort middagen") {
		t.Error("typed dinner not offered for editing")
	}

	if rec := post(t, h, "/week/day", dayForm("3", url.Values{"recipe": {itoa(id)}})); rec.Code != http.StatusSeeOther {
		t.Fatalf("set recipe: %d", rec.Code)
	}
	if e, _ := st.plans[_w40].Entry(3); e.RecipeID != id || !e.Locked || e.Servings < 1 {
		t.Fatalf("recipe entry = %+v", e)
	}

	if rec := post(t, h, "/week/day", dayForm("2", url.Values{"clear": {"1"}})); rec.Code != http.StatusSeeOther {
		t.Fatalf("clear: %d", rec.Code)
	}
	if _, ok := st.plans[_w40].Entry(2); ok {
		t.Fatal("day 2 still has a dinner")
	}
	if rec := post(t, h, "/week/day", dayForm("2", url.Values{"clear": {"1"}})); rec.Code != http.StatusBadRequest {
		t.Fatalf("clear an empty day: %d", rec.Code)
	}
}

// Review focus 1 and 2: blank text is required, and length counts letters, not bytes.
func TestTypedDinnerValidation(t *testing.T) {
	st := newFakeStore()
	h := newServer(t, i18n.SV, true, st)
	for title, want := range map[string]string{
		"   ":                   "Obligatoriskt",
		"\n":                    "Obligatoriskt",
		strings.Repeat("ä", 61): "För långt",
	} {
		rec := post(t, h, "/week/day", dayForm("1", url.Values{"title": {title}}))
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("title %q: %d", title, rec.Code)
		}
	}
	if _, ok := st.plans[_w40]; ok {
		t.Fatal("a refused dinner created the week")
	}
	if rec := post(t, h, "/week/day", dayForm("1", url.Values{"title": {strings.Repeat("ä", 60)}})); rec.Code != http.StatusSeeOther {
		t.Fatalf("60 letters: %d", rec.Code)
	}
}

// Review focus 3 and 4: only planning-window drafts, existing recipes and days with dinner
// at home.
func TestSetDinnerRefusals(t *testing.T) {
	st := newFakeStore()
	st.settings.DinnersPerWeek = 5
	h := newServer(t, i18n.SV, true, st)
	for _, q := range []string{"day=6", "day=9", "day=x", "day=0"} {
		if res, _ := get(t, h, "/week/day?y=2026&w=40&"+q); res.StatusCode != http.StatusBadRequest {
			t.Errorf("GET %s: %d", q, res.StatusCode)
		}
	}
	if res, _ := get(t, h, "/week/day?y=2026&w=30&day=1"); res.StatusCode != http.StatusBadRequest {
		t.Errorf("past week: %d", res.StatusCode)
	}
	if rec := post(t, h, "/week/day", dayForm("1", url.Values{"recipe": {"999"}})); rec.Code != http.StatusBadRequest {
		t.Errorf("unknown recipe: %d", rec.Code)
	}
	if rec := post(t, h, "/week/day", dayForm("1", url.Values{"recipe": {"x"}})); rec.Code != http.StatusBadRequest {
		t.Errorf("bad recipe id: %d", rec.Code)
	}
	// Monday is at home by default: the dinner creates the week with the household's days.
	if rec := post(t, h, "/week/day", dayForm("1", url.Values{"title": {"Gäster"}})); rec.Code != http.StatusSeeOther {
		t.Fatalf("set Monday: %d", rec.Code)
	}
	if p := st.plans[_w40]; !p.Context.Days[5].Skip {
		t.Fatalf("new week context = %+v", p.Context.Days)
	}
	// A stale page posting to a day switched to "no dinner at home".
	c := st.plans[_w40].Context
	c.Days[1].Skip = true
	if err := st.SaveContext(t.Context(), _w40, c, [7]int{}); err != nil {
		t.Fatal(err)
	}
	if rec := post(t, h, "/week/day", dayForm("2", url.Values{"title": {"Gäster"}})); rec.Code != http.StatusBadRequest {
		t.Errorf("no-dinner day: %d", rec.Code)
	}
	if _, ok := st.plans[_w40].Entry(2); ok {
		t.Error("dinner stored on a no-dinner day")
	}
}

// Approved weeks refuse with 409, and a running plan refuses with 400.
func TestSetDinnerNeedsAnIdleDraft(t *testing.T) {
	st := newFakeStore()
	pl := &fakePlanner{st: st, gate: make(chan struct{})}
	h, s := newPlanningServer(t, i18n.SV, st, pl)
	post(t, h, "/week/generate", weekForm(nil))
	if rec := post(t, h, "/week/day", dayForm("3", url.Values{"title": {"Gäster"}})); rec.Code != http.StatusBadRequest {
		t.Errorf("while planning: %d", rec.Code)
	}
	close(pl.gate)
	s.jobs.wait()
	if rec := post(t, h, "/week/approve", weekForm(nil)); rec.Code != http.StatusSeeOther {
		t.Fatalf("approve: %d", rec.Code)
	}
	if rec := post(t, h, "/week/day", dayForm("3", url.Values{"title": {"Gäster"}})); rec.Code != http.StatusConflict {
		t.Errorf("approved week: %d", rec.Code)
	}
	if res, _ := get(t, h, "/week/day?y=2026&w=40&day=3"); res.StatusCode != http.StatusConflict {
		t.Errorf("GET approved week: %d", res.StatusCode)
	}
}
