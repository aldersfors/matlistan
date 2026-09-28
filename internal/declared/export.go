package declared

import (
	"sigs.k8s.io/yaml"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/recipes"
)

// MemberYAML is a member as a one-item list for members: in the household file. The login
// link and the id stay in the app.
func MemberYAML(m household.Member) (string, error) {
	b, err := yaml.Marshal([]fileMember{{Key: m.Key, Name: m.Name, BirthYear: m.BirthYear,
		Diets: m.Diets, Allergens: m.Allergens, Likes: m.Likes, Dislikes: m.Dislikes}})
	return string(b), err
}

// RecipeYAML is a recipe as a one-item list for recipes: in the household file.
func RecipeYAML(r recipes.Recipe) (string, error) {
	fr := fileRecipe{Key: r.Key, Title: r.Title, Lang: string(r.Lang), Servings: r.Servings,
		ActiveMinutes: r.ActiveMinutes, TotalMinutes: r.TotalMinutes, Description: r.Description,
		Tags: r.Tags, Diets: r.Diets, Allergens: r.Allergens, SourceURL: r.SourceURL,
		Steps: r.Steps, Ingredients: []fileIngredient{}}
	for _, in := range r.Ingredients {
		fr.Ingredients = append(fr.Ingredients, fileIngredient{Name: in.Name,
			Quantity: in.Quantity, Unit: in.Unit, Section: in.Section, Optional: in.Optional})
	}
	b, err := yaml.Marshal([]fileRecipe{fr})
	return string(b), err
}
