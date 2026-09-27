package web

import (
	"math"
	"net/http"
	"strings"

	"github.com/aldersfors/matlistan/internal/i18n"

	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/shopping"
	"github.com/aldersfors/matlistan/internal/web/views"
)

const _queryRunesMax = 100

func (s *server) recipeList(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("q")
	if !storable(raw) {
		http.Error(w, i18n.T(r.Context(), "error.bad_request"), http.StatusBadRequest)
		return
	}
	v, err := s.recipeListView(r, raw)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.RecipeListPage(v))
}

// recipeListView is the library for query raw (trimmed and capped).
func (s *server) recipeListView(r *http.Request, raw string) (views.RecipeList, error) {
	q := []rune(strings.TrimSpace(raw))
	q = q[:min(len(q), _queryRunesMax)]
	list, err := s.Store.ListRecipes(r.Context(), s.Catalog.Locale(), string(q))
	if err != nil {
		return views.RecipeList{}, err
	}
	v := views.RecipeList{Query: string(q), CanImport: s.Importer != nil}
	for _, it := range list {
		v.Items = append(v.Items, views.RecipeItem{ID: it.ID, Title: it.Title,
			Meta:   s.Catalog.T("recipes.minutes", "n", it.TotalMinutes),
			Rating: s.ratingText(it.Rating)})
	}
	return v, nil
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
	servings, day := servingsParam(r, rec.Servings), dayParam(r)
	v := views.RecipeView{ID: rec.ID, Title: rec.Title, Description: rec.Description,
		Meta: c.T("recipes.minutes", "n", rec.TotalMinutes) + ", " +
			c.N("recipes.servings", servings),
		Steps: rec.Steps, CookHref: cookHref(rec.ID, servings, day)}
	if rt := s.ratingText(rec.Rating); rt != "" {
		v.Meta += ", " + rt
	}
	if rec.SourceURL != "" {
		v.SourceHref = rec.SourceURL
		v.Source = s.Catalog.T("recipes.source", "host", hostOf(rec.SourceURL))
	}
	for _, d := range rec.Diets {
		v.Chips = append(v.Chips, c.Diet(d))
	}
	for _, a := range rec.Allergens {
		v.Chips = append(v.Chips, c.Allergen(a))
	}
	for _, in := range rec.Scaled(servings) {
		v.Ingredients = append(v.Ingredients, s.ingredientLine(in))
	}
	s.render(w, r, http.StatusOK, views.RecipePage(v))
}

// ratingText is "3,7 av 5, 3 betyg", or "" for an unrated recipe.
func (s *server) ratingText(r recipes.Rating) string {
	if r.Count == 0 {
		return ""
	}
	avg := s.Catalog.Quantity(math.Round(r.Average*10) / 10)
	return s.Catalog.N("recipes.rating", r.Count, "avg", avg)
}

// ingredientLine is "1,5 dl vispgrädde", or "salt, to taste (optional)".
func (s *server) ingredientLine(in recipes.Ingredient) string {
	return shopping.Line(s.Catalog, in.Name, in.Quantity, in.Unit, in.Optional)
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
