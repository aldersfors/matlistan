package web

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/recipes"
)

func meatballs() url.Values {
	return url.Values{
		"title": {"Köttbullar"}, "servings": {"4"}, "total_minutes": {"40"},
		"active_minutes": {"30"}, "tags": {"vardag, favorit"},
		"steps":                  {"Blanda smeten\r\n\r\nRulla och stek"},
		"allergens":              {"milk"},
		"ingredients.0.name":     {"blandfärs"},
		"ingredients.0.quantity": {"500"},
		"ingredients.0.unit":     {"g"},
		"ingredients.0.section":  {"meat_fish"},
		"ingredients.1.name":     {"vispgrädde"},
		"ingredients.1.quantity": {"1,5"},
		"ingredients.1.unit":     {"dl"},
		"ingredients.1.section":  {"dairy"},
		"ingredients.2.name":     {""},
		"ingredients.2.quantity": {""},
		"ingredients.2.section":  {"produce"},
		"ingredients.3.name":     {"salt"},
		"ingredients.3.section":  {"pantry"},
		"ingredients.3.optional": {"on"},
	}
}

func TestCreateAndShowRecipe(t *testing.T) {
	st := newFakeStore()
	h := newServer(t, i18n.SV, true, st)
	rec := post(t, h, "/recipes", meatballs())
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/recipes/1" {
		t.Fatalf("status %d location %q body %.300s", rec.Code, rec.Header().Get("Location"),
			rec.Body.String())
	}
	r, _ := st.GetRecipe(t.Context(), 1)
	if len(r.Ingredients) != 3 || r.Lang != i18n.SV || r.Source != "manual" ||
		len(r.Steps) != 2 || r.Ingredients[1].Quantity != 1.5 {
		t.Fatalf("stored %+v", r)
	}
	_, body := get(t, h, "/recipes/1")
	for _, want := range []string{"Köttbullar", "40 min", "4 portioner", "1,5 dl vispgrädde",
		"500 g blandfärs", "salt, efter smak (valfri)", "Mjölk", "Rulla och stek"} {
		if !strings.Contains(body, want) {
			t.Errorf("recipe page lacks %q", want)
		}
	}
}

// Review focus 2: an error on raw row 3 stays on row 3, with the typed text kept.
func TestRecipeErrorsStayOnTheirRow(t *testing.T) {
	h := newServer(t, i18n.SV, true, newFakeStore())
	v := meatballs()
	v.Set("ingredients.3.quantity", "lite")
	rec := post(t, h, "/recipes", v)
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d", rec.Code)
	}
	if !strings.Contains(body, `name="ingredients.3.quantity" value="lite"`) {
		t.Error("typed quantity not kept on row 3")
	}
	i := strings.Index(body, `name="ingredients.3.quantity"`)
	if i < 0 || !strings.Contains(body[i:], "Inte ett tal") {
		t.Error("error not shown after row 3's field")
	}
	if !strings.Contains(body, `value="Köttbullar"`) {
		t.Error("title not kept")
	}
}

func TestRecipeUnitRequired(t *testing.T) {
	h := newServer(t, i18n.SV, true, newFakeStore())
	v := meatballs()
	v.Set("ingredients.1.unit", "")
	if rec := post(t, h, "/recipes", v); !strings.Contains(rec.Body.String(), "Välj en enhet") {
		t.Fatalf("status %d, no unit error", rec.Code)
	}
}

// Review focus 1: search folds Swedish letters; the list follows the deployment language.
func TestRecipeListAndSearch(t *testing.T) {
	st := newFakeStore()
	for _, r := range []recipes.Recipe{{Title: "Ärtsoppa", Lang: i18n.SV, TotalMinutes: 40},
		{Title: "Köttbullar", Lang: i18n.SV, TotalMinutes: 40},
		{Title: "Pea soup", Lang: i18n.EN, TotalMinutes: 40}} {
		if _, err := st.CreateRecipe(t.Context(), r); err != nil {
			t.Fatal(err)
		}
	}
	h := newServer(t, i18n.SV, true, st)
	_, body := get(t, h, "/recipes")
	if !strings.Contains(body, "Ärtsoppa") || !strings.Contains(body, "Köttbullar") ||
		strings.Contains(body, "Pea soup") {
		t.Fatal("list does not follow the deployment language")
	}
	_, body = get(t, h, "/recipes?q=%C3%84RT")
	if !strings.Contains(body, "Ärtsoppa") || strings.Contains(body, "Köttbullar") {
		t.Fatal("search for ÄRT")
	}
	_, body = get(t, h, "/recipes?q=zzz")
	if !strings.Contains(body, `Inga recept matchar &#34;zzz&#34;.`) &&
		!strings.Contains(body, `Inga recept matchar "zzz".`) {
		t.Fatal("no-match message missing")
	}
}

func TestEditAndArchiveRecipe(t *testing.T) {
	st := newFakeStore()
	h := newServer(t, i18n.SV, true, st)
	post(t, h, "/recipes", meatballs())
	_, body := get(t, h, "/recipes/1/edit")
	if !strings.Contains(body, `value="blandfärs"`) || !strings.Contains(body, "ingredients.5.name") {
		t.Fatal("edit form lacks existing rows plus three blank ones")
	}
	v := meatballs()
	v.Set("title", "Mormors köttbullar")
	if rec := post(t, h, "/recipes/1", v); rec.Code != http.StatusSeeOther {
		t.Fatalf("update: %d", rec.Code)
	}
	if r, _ := st.GetRecipe(t.Context(), 1); r.Title != "Mormors köttbullar" {
		t.Fatalf("title = %q", r.Title)
	}
	if rec := post(t, h, "/recipes/1/archive", nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("archive: %d", rec.Code)
	}
	if res, _ := get(t, h, "/recipes/1"); res.StatusCode != http.StatusNotFound {
		t.Fatalf("archived recipe: %d", res.StatusCode)
	}
}

// Phones must offer a decimal keypad for amounts ("1,5"); whole-number fields keep numeric.
func TestQuantityFieldAllowsDecimals(t *testing.T) {
	_, body := get(t, newServer(t, i18n.SV, true, newFakeStore()), "/recipes/new")
	if !strings.Contains(body, `inputmode="decimal" name="ingredients.0.quantity"`) {
		t.Error("quantity field lacks inputmode=decimal")
	}
	if !strings.Contains(body, `inputmode="numeric" name="servings"`) {
		t.Error("servings field lost inputmode=numeric")
	}
}

// Ratings show on the library and the recipe, in the locale's number format; unrated
// recipes show nothing.
func TestRecipesShowTheirRating(t *testing.T) {
	st := newFakeStore()
	h := newServer(t, i18n.SV, true, st)
	rated, _ := st.CreateRecipe(t.Context(), recipes.Recipe{Title: "Ärtsoppa", Lang: i18n.SV,
		Servings: 4, TotalMinutes: 40, Source: "manual"})
	_, _ = st.CreateRecipe(t.Context(), recipes.Recipe{Title: "Pannkakor", Lang: i18n.SV,
		Servings: 4, TotalMinutes: 30, Source: "manual"})
	r := st.recipes[rated]
	r.Rating = recipes.Rating{Average: 11.0 / 3, Count: 3}
	st.recipes[rated] = r

	_, list := get(t, h, "/recipes")
	if !strings.Contains(list, "3,7 av 5, 3 betyg") {
		t.Errorf("library lacks the rating: %s", list)
	}
	if strings.Count(list, "av 5") != 1 {
		t.Error("an unrated recipe shows a rating")
	}
	if _, page := get(t, h, fmt.Sprintf("/recipes/%d", rated)); !strings.Contains(page,
		"3,7 av 5, 3 betyg") {
		t.Error("recipe page lacks the rating")
	}
}
