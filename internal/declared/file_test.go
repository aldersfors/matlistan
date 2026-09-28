package declared

import (
	"strings"
	"testing"
	"time"

	"github.com/aldersfors/matlistan/internal/i18n"
)

var _now = time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)

func id(s string) string { return s }

const _good = `
members:
  - key: anna
    name: Anna
    birthYear: 1985
    diets: [vegetarian]
    allergens: [peanuts]
    likes: tacos
recipes:
  - key: pumpasoppa
    title: Pumpasoppa
    servings: 4
    activeMinutes: 20
    totalMinutes: 40
    steps: ["Koka."]
    ingredients:
      - {name: pumpa, quantity: 1, unit: kg, section: produce}
      - {name: salt, section: pantry}
recipeURLs:
  - https://www.arla.se/recept/pannkaka/#top
`

func TestParseAValidFile(t *testing.T) {
	h, err := Parse([]byte(_good), i18n.SV, _now, id)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.Members) != 1 || h.Members[0].Key != "anna" || h.Members[0].Managed != "inline" ||
		h.Members[0].BirthYear != 1985 {
		t.Fatalf("members = %+v", h.Members)
	}
	r := h.Recipes[0]
	if r.Key != "pumpasoppa" || r.Lang != i18n.SV || r.Source != "manual" || r.Managed != "inline" ||
		len(r.Ingredients) != 2 || r.Ingredients[1].Quantity != 0 || r.Ingredients[1].Unit != "" {
		t.Fatalf("recipe = %+v", r)
	}
	if len(h.RecipeURLs) != 1 || h.RecipeURLs[0] != "https://www.arla.se/recept/pannkaka" {
		t.Fatalf("links = %v", h.RecipeURLs)
	}
}

// Every problem is reported at once, named by list and key.
func TestParseReportsEveryProblem(t *testing.T) {
	bad := `
members:
  - {key: Anna, name: "", birthYear: 1800}
  - {key: bo, name: Bo, birthYear: 2010, allergens: [dust]}
  - {key: bo, name: Bo 2, birthYear: 2012}
recipes:
  - {key: soppa, title: Soppa, servings: 0, totalMinutes: 0, steps: [], ingredients: []}
  - {key: gryta, title: Gryta, servings: 4, totalMinutes: 30, steps: [x], sourceURL: "https://a.example/r",
     ingredients: [{name: lök, quantity: 1, unit: bucket, section: produce}]}
recipeURLs: ["http://a.example/insecure", "https://a.example/r", "https://b.example/x", "https://b.example/x"]
`
	_, err := Parse([]byte(bad), i18n.SV, _now, id)
	if err == nil {
		t.Fatal("bad file accepted")
	}
	for _, want := range []string{
		`members[Anna].key`, `members[Anna].name`, `members[Anna].birth_year`,
		`members[bo].allergens`, `members[bo]: key used twice`,
		`recipes[soppa].servings`, `recipes[soppa].total_minutes`,
		`recipes[gryta].ingredients.0.unit`,
		`recipeURLs[0]`, `recipeURLs[1]: also a recipe's sourceURL`,
		`recipeURLs[3]: listed twice`,
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q:\n%v", want, err)
		}
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	if _, err := Parse([]byte("members:\n  - {key: a, name: A, birthYear: 1990, age: 36}\n"), i18n.SV, _now, id); err == nil ||
		!strings.Contains(err.Error(), "age") {
		t.Fatalf("unknown field: %v", err)
	}
}
