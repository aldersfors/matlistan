package planner

import (
	"strings"
	"testing"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/recipes"
)

func ptr(v int64) *int64 { return &v }

func newSoup(title string) *NewRecipe {
	return &NewRecipe{Title: title, Servings: 4, ActiveMinutes: 15, TotalMinutes: 30,
		Steps: []string{"Koka"}, Diets: []string{"vegetarian"},
		Ingredients: []NewIngredient{{Name: "pumpa", Quantity: 1, Unit: "kg", Section: "produce"},
			{Name: "salt", Section: "pantry"}}}
}

// weekRequest plans Monday and Tuesday only, Tuesday is a quick evening.
func weekRequest() Request {
	r := sampleRequest()
	for i := range r.Days {
		r.Days[i].Planned = i < 2
	}
	r.Days[1].MaxMinutes = 20
	return r
}

func library() map[int64]recipes.Recipe {
	return map[int64]recipes.Recipe{7: {ID: 7, Title: "Ärtsoppa", TotalMinutes: 20,
		Ingredients: []recipes.Ingredient{{Name: "gul ärtsoppa", Quantity: 2, Unit: "pcs",
			Section: "pantry"}}}}
}

func TestResolveAcceptsAGoodWeek(t *testing.T) {
	p := Proposal{Days: []ProposedDay{
		{Day: 1, Why: "Pumpan är i säsong.", NewRecipe: newSoup("Pumpasoppa")},
		{Day: 2, Why: "Snabb torsdagsklassiker.", LibraryRecipeID: ptr(7)},
	}}
	picks, vs := Resolve(weekRequest(), p, library())
	if len(vs) != 0 {
		t.Fatalf("violations: %v", vs)
	}
	if len(picks) != 2 || picks[0].New == nil || picks[0].New.Source != "generated" ||
		picks[0].New.Lang != i18n.SV || picks[1].RecipeID != 7 || picks[0].Servings != 2 { // one adult + a 12-year-old: 1.75 -> 2
		t.Fatalf("picks = %+v", picks)
	}
}

// Review focus 1 and 2: every kind of broken answer becomes a violation, never a panic.
func TestResolveFindsViolations(t *testing.T) {
	cases := map[string]struct {
		days []ProposedDay
		want string
	}{
		"missing day": {[]ProposedDay{{Day: 1, Why: "x", LibraryRecipeID: ptr(7)}},
			"day 2: no dinner planned"},
		"extra day": {[]ProposedDay{{Day: 1, Why: "x", LibraryRecipeID: ptr(7)},
			{Day: 2, Why: "x", NewRecipe: newSoup("A")}, {Day: 5, Why: "x", NewRecipe: newSoup("B")}},
			"day 5: not a planned day"},
		"bad day number": {[]ProposedDay{{Day: 9, Why: "x", LibraryRecipeID: ptr(7)}},
			"day 9: not a planned day"},
		"both chosen": {[]ProposedDay{{Day: 1, Why: "x", LibraryRecipeID: ptr(7),
			NewRecipe: newSoup("A")}, {Day: 2, Why: "x", NewRecipe: newSoup("B")}},
			"day 1: set exactly one of library_recipe_id and new_recipe"},
		"unknown library id": {[]ProposedDay{{Day: 1, Why: "x", LibraryRecipeID: ptr(99)},
			{Day: 2, Why: "x", LibraryRecipeID: ptr(7)}}, "day 1: 99 is not a candidate id"},
		"no why": {[]ProposedDay{{Day: 1, Why: " ", NewRecipe: newSoup("A")},
			{Day: 2, Why: "x", LibraryRecipeID: ptr(7)}}, "day 1: why is empty"},
		"too slow": {[]ProposedDay{{Day: 1, Why: "x", LibraryRecipeID: ptr(7)},
			{Day: 2, Why: "x", NewRecipe: newSoup("A")}}, "day 2: takes 30 minutes, the limit is 20"},
		"repeat recent": {[]ProposedDay{{Day: 1, Why: "x", NewRecipe: newSoup("köttbullar")},
			{Day: 2, Why: "x", LibraryRecipeID: ptr(7)}}, "day 1: \"köttbullar\" was cooked recently"},
		"same twice": {[]ProposedDay{{Day: 1, Why: "x", LibraryRecipeID: ptr(7)},
			{Day: 2, Why: "x", LibraryRecipeID: ptr(7)}}, "day 2: \"Ärtsoppa\" is already planned this week"},
		"declared allergen": {[]ProposedDay{{Day: 1, Why: "x", NewRecipe: func() *NewRecipe {
			n := newSoup("A")
			n.Allergens = []string{"nuts"}
			return n
		}()}, {Day: 2, Why: "x", LibraryRecipeID: ptr(7)}}, "day 1: contains nuts"},
		"hidden allergen": {[]ProposedDay{{Day: 1, Why: "x", NewRecipe: func() *NewRecipe {
			n := newSoup("A")
			n.Ingredients = append(n.Ingredients, NewIngredient{Name: "rostade hasselnötter",
				Quantity: 50, Unit: "g", Section: "pantry"})
			return n
		}()}, {Day: 2, Why: "x", LibraryRecipeID: ptr(7)}}, "day 1: \"rostade hasselnötter\" suggests nuts"},
		"broken recipe": {[]ProposedDay{{Day: 1, Why: "x", NewRecipe: func() *NewRecipe {
			n := newSoup("A")
			n.Ingredients[0].Unit = ""
			return n
		}()}, {Day: 2, Why: "x", LibraryRecipeID: ptr(7)}}, "day 1: ingredients.0.unit: form.unit_required"},
	}
	for name, c := range cases {
		_, vs := Resolve(weekRequest(), Proposal{Days: c.days}, library())
		if !strings.Contains(Feedback(vs), c.want) {
			t.Errorf("%s: feedback %q lacks %q", name, Feedback(vs), c.want)
		}
	}
}

func TestDietsAndPork(t *testing.T) {
	r := weekRequest()
	r.Allergens = nil
	r.Diets = []string{"no_pork"}
	n := newSoup("A")
	n.Diets = nil
	n.Ingredients = append(n.Ingredients, NewIngredient{Name: "bacon", Quantity: 100, Unit: "g",
		Section: "meat_fish"})
	_, vs := Resolve(r, Proposal{Days: []ProposedDay{{Day: 1, Why: "x", NewRecipe: n},
		{Day: 2, Why: "x", LibraryRecipeID: ptr(7)}}}, library())
	fb := Feedback(vs)
	if !strings.Contains(fb, "day 1: does not declare no_pork") ||
		!strings.Contains(fb, "day 1: \"bacon\" suggests pork") {
		t.Fatalf("feedback = %q", fb)
	}
	for _, c := range []struct {
		declared []string
		required string
		want     bool
	}{{[]string{"vegan"}, "vegetarian", true}, {[]string{"vegetarian"}, "vegan", false},
		{[]string{"pescatarian"}, "no_pork", true}, {[]string{"vegetarian"}, "pescatarian", true},
		{nil, "vegan", false}} {
		if suitsDiet(c.declared, c.required) != c.want {
			t.Errorf("suitsDiet(%v, %s) != %v", c.declared, c.required, c.want)
		}
	}
}

func TestAllergenKeywords(t *testing.T) {
	cases := []struct {
		allergen, name string
		want           bool
	}{
		{"nuts", "hasselnötter", true}, {"nuts", "nötfärs", false}, {"nuts", "cashewnötter", true},
		{"milk", "riven ost", true}, {"milk", "fetaost", true}, {"milk", "rostade kikärtor", false},
		{"milk", "vispgrädde", true}, {"eggs", "ägg", true}, {"eggs", "äggula", true},
		{"fish", "laxfilé", true}, {"gluten", "vetemjöl", true}, {"sesame", "tahini", true},
		{"crustaceans", "räkor", true}, {"peanuts", "jordnötssmör", true},
	}
	for _, c := range cases {
		if got := containsAllergen(i18n.SV, c.allergen, c.name); got != c.want {
			t.Errorf("%s in %q = %v", c.allergen, c.name, got)
		}
	}
	if !containsAllergen(i18n.EN, "milk", "grated cheese") ||
		containsAllergen(i18n.EN, "nuts", "coconut milk") {
		t.Error("en keywords")
	}
}
