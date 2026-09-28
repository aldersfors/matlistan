package web

import (
	"html"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aldersfors/matlistan/internal/declared"
	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/recipes"
)

func managedStore(t *testing.T) (*fakeStore, int64, int64) {
	t.Helper()
	st := newFakeStore()
	mid, _ := st.CreateMember(t.Context(), household.Member{Name: "Anna", BirthYear: 1985,
		Key: "anna", Managed: "inline"})
	rid, _ := st.CreateRecipe(t.Context(), recipes.Recipe{Title: "Pumpasoppa", Lang: i18n.SV,
		Servings: 4, TotalMinutes: 40, Steps: []string{"Koka."}, Source: "manual",
		Key: "pumpasoppa", Managed: "inline",
		Ingredients: []recipes.Ingredient{{Name: "pumpa", Quantity: 1, Unit: "kg", Section: "produce"}}})
	return st, mid, rid
}

func TestManagedRowsAreLocked(t *testing.T) {
	st, mid, rid := managedStore(t)
	h := newServer(t, i18n.SV, true, st)
	_, body := get(t, h, "/recipes/"+itoa(rid))
	if !strings.Contains(body, "Hanteras i git") || strings.Contains(body, "/recipes/"+itoa(rid)+"/edit") {
		t.Fatalf("recipe page: %.600s", body)
	}
	_, body = get(t, h, "/family")
	if !strings.Contains(body, "Hanteras i git") || strings.Contains(body, "/family/"+itoa(mid)+"/edit") {
		t.Fatalf("family page: %.600s", body)
	}
	if res, _ := get(t, h, "/recipes/"+itoa(rid)+"/edit"); res.StatusCode != http.StatusConflict {
		t.Errorf("recipe edit: %d", res.StatusCode)
	}
	if res, _ := get(t, h, "/family/"+itoa(mid)+"/edit"); res.StatusCode != http.StatusConflict {
		t.Errorf("member edit: %d", res.StatusCode)
	}
	for _, path := range []string{"/recipes/" + itoa(rid), "/recipes/" + itoa(rid) + "/archive",
		"/family/" + itoa(mid), "/family/" + itoa(mid) + "/archive"} {
		if rec := post(t, h, path, url.Values{"name": {"X"}, "title": {"X"}}); rec.Code != http.StatusConflict {
			t.Errorf("POST %s: %d", path, rec.Code)
		}
	}
	// "Det här är jag" still works on a managed member.
	if rec := post(t, h, "/family/"+itoa(mid)+"/me", nil); rec.Code != http.StatusSeeOther {
		t.Errorf("link: %d", rec.Code)
	}
}

// Review focus 4: what "Kopiera som YAML" shows parses back into the same recipe and member.
func TestCopyAsYAMLParsesBack(t *testing.T) {
	st, _, rid := managedStore(t)
	h := newServer(t, i18n.SV, true, st)
	_, body := get(t, h, "/recipes/"+itoa(rid))
	y := between(t, body, `<pre id="yaml-recipe"`, "</pre>")
	y = y[strings.Index(y, ">")+1:]
	hh, err := declared.Parse([]byte("recipes:\n"+indentLines(unescape(y))), i18n.SV, fixedNow(), func(s string) string { return s })
	if err != nil || hh.Recipes[0].Title != "Pumpasoppa" || hh.Recipes[0].Key != "pumpasoppa" {
		t.Fatalf("recipe YAML: %v\n%s", err, y)
	}
	_, body = get(t, h, "/family")
	y = between(t, body, `<pre id="yaml-member-`, "</pre>")
	y = y[strings.Index(y, ">")+1:]
	hh, err = declared.Parse([]byte("members:\n"+indentLines(unescape(y))), i18n.SV, fixedNow(), func(s string) string { return s })
	if err != nil || hh.Members[0].Name != "Anna" {
		t.Fatalf("member YAML: %v\n%s", err, y)
	}
	if !strings.Contains(body, `src="/static/copy.js"`) {
		t.Fatal("copy script not included")
	}
}

func TestAppRowsStayEditable(t *testing.T) {
	st := newFakeStore()
	rid, _ := st.CreateRecipe(t.Context(), recipes.Recipe{Title: "Egen", Lang: i18n.SV, Servings: 2,
		TotalMinutes: 10, Steps: []string{"x"}, Source: "manual", Key: "egen",
		Ingredients: []recipes.Ingredient{{Name: "x", Section: "other"}}})
	h := newServer(t, i18n.SV, true, st)
	if _, body := get(t, h, "/recipes/"+itoa(rid)); !strings.Contains(body, "/recipes/"+itoa(rid)+"/edit") ||
		strings.Contains(body, "Hanteras i git") || !strings.Contains(body, "Kopiera som YAML") {
		t.Fatal("app recipe should be editable and copyable")
	}
}

func between(t *testing.T, s, start, end string) string {
	t.Helper()
	i := strings.Index(s, start)
	if i < 0 {
		t.Fatalf("no %q in %.400s", start, s)
	}
	rest := s[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		t.Fatalf("no %q after %q", end, start)
	}
	return rest[:j]
}

func indentLines(s string) string {
	return "  " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n  ") + "\n"
}

func unescape(s string) string { return html.UnescapeString(s) }

func fixedNow() time.Time {
	sthlm, _ := time.LoadLocation("Europe/Stockholm")
	return time.Date(2026, 9, 27, 8, 0, 0, 0, sthlm)
}
