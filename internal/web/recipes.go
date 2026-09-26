package web

import (
	"net/http"
	"strings"

	"github.com/jalet/matlistan/internal/i18n"

	"github.com/jalet/matlistan/internal/recipes"
	"github.com/jalet/matlistan/internal/web/views"
)

const _queryRunesMax = 100

func (s *server) recipeList(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("q")
	if !storable(raw) {
		http.Error(w, i18n.T(r.Context(), "error.bad_request"), http.StatusBadRequest)
		return
	}
	q := []rune(strings.TrimSpace(raw))
	q = q[:min(len(q), _queryRunesMax)]
	list, err := s.Store.ListRecipes(r.Context(), s.Catalog.Locale(), string(q))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v := views.RecipeList{Query: string(q)}
	for _, it := range list {
		v.Items = append(v.Items, views.RecipeItem{ID: it.ID, Title: it.Title,
			Meta: s.Catalog.T("recipes.minutes", "n", it.TotalMinutes)})
	}
	s.render(w, r, http.StatusOK, views.RecipeListPage(v))
}

func (s *server) showRecipe(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	rec, err := s.Store.GetRecipe(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c := s.Catalog
	v := views.RecipeView{ID: rec.ID, Title: rec.Title, Description: rec.Description,
		Meta: c.T("recipes.minutes", "n", rec.TotalMinutes) + ", " +
			c.N("recipes.servings", rec.Servings),
		Steps: rec.Steps}
	for _, d := range rec.Diets {
		v.Chips = append(v.Chips, c.Diet(d))
	}
	for _, a := range rec.Allergens {
		v.Chips = append(v.Chips, c.Allergen(a))
	}
	for _, in := range rec.Ingredients {
		v.Ingredients = append(v.Ingredients, s.ingredientLine(in))
	}
	s.render(w, r, http.StatusOK, views.RecipePage(v))
}

// ingredientLine is "1,5 dl vispgrädde", or "salt, to taste (optional)".
func (s *server) ingredientLine(in recipes.Ingredient) string {
	c := s.Catalog
	line := in.Name + ", " + c.T("recipe.to_taste")
	if in.Quantity > 0 {
		line = strings.TrimSpace(c.Quantity(in.Quantity)+" "+c.Unit(in.Unit)) + " " + in.Name
	}
	if in.Optional {
		line += " (" + c.T("recipe.optional") + ")"
	}
	return line
}

func (s *server) archiveRecipe(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	if err := s.Store.ArchiveRecipe(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/recipes", http.StatusSeeOther)
}
