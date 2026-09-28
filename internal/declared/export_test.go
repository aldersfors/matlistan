package declared

import (
	"strings"
	"testing"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/keys"
	"github.com/aldersfors/matlistan/internal/recipes"
)

func TestMemberYAMLRoundTrips(t *testing.T) {
	m := household.Member{Key: "anna", Name: "Anna", BirthYear: 1985, Diets: []string{"vegetarian"},
		Allergens: []string{"peanuts"}, Likes: "tacos", Subject: "sub-secret", ID: 7}
	y, err := MemberYAML(m)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(y, "sub-secret") || strings.Contains(y, "id:") {
		t.Fatalf("YAML leaks app-only fields:\n%s", y)
	}
	h, err := Parse([]byte("members:\n"+indent(y)), i18n.SV, _now, id)
	if err != nil || len(h.Members) != 1 || h.Members[0].Name != "Anna" || h.Members[0].Likes != "tacos" {
		t.Fatalf("round trip: %+v, %v\n%s", h.Members, err, y)
	}
}

// Review focus 4: a long Swedish title's key and YAML parse back into the same recipe.
func TestExportRoundTripsLongTitles(t *testing.T) {
	title := "Långkokt högrevsgryta med rotfrukter, äpple och örter från trädgården i september"
	r := recipes.Recipe{Key: keys.Slug(title) + "-2", Title: title, Lang: i18n.SV, Servings: 6,
		ActiveMinutes: 30, TotalMinutes: 240, Steps: []string{"Bryn köttet."},
		SourceURL: "https://www.ica.se/recept/x-123",
		Ingredients: []recipes.Ingredient{{Name: "högrev", Quantity: 1.2, Unit: "kg", Section: "meat_fish"},
			{Name: "salt", Section: "pantry"}, {Name: "timjan", Quantity: 1, Unit: "tsp", Section: "pantry", Optional: true}}}
	y, err := RecipeYAML(r)
	if err != nil {
		t.Fatal(err)
	}
	h, err := Parse([]byte("recipes:\n"+indent(y)), i18n.SV, _now, id)
	if err != nil {
		t.Fatalf("does not parse: %v\n%s", err, y)
	}
	got := h.Recipes[0]
	if got.Key != r.Key || got.Title != title || got.TotalMinutes != 240 || len(got.Ingredients) != 3 ||
		!got.Ingredients[2].Optional || got.SourceURL != r.SourceURL {
		t.Fatalf("round trip = %+v\n%s", got, y)
	}
}

func indent(s string) string {
	return "  " + strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n  ") + "\n"
}
