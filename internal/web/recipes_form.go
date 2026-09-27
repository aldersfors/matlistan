package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/store"
	"github.com/aldersfors/matlistan/internal/validate"
	"github.com/aldersfors/matlistan/internal/web/views"
)

// _blankRows are offered after the used rows; _rowsMax bounds how many rows are read.
const (
	_blankRows = 3
	_rowsMax   = recipes.IngredientsMax + _blankRows
)

func (s *server) newRecipe(w http.ResponseWriter, r *http.Request) {
	f := s.recipeForm(recipes.Recipe{Servings: 4}, nil)
	f.Servings, f.TotalMinutes, f.ActiveMinutes = "4", "", ""
	s.render(w, r, http.StatusOK, views.RecipeFormPage(f))
}

func (s *server) editRecipe(w http.ResponseWriter, r *http.Request) {
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
	s.render(w, r, http.StatusOK, views.RecipeFormPage(s.recipeForm(rec, nil)))
}

func (s *server) createRecipe(w http.ResponseWriter, r *http.Request) {
	if !readForm(w, r) {
		return
	}
	rec, e := s.parseRecipe(r)
	if len(e) > 0 {
		s.render(w, r, http.StatusUnprocessableEntity, views.RecipeFormPage(s.rawRecipeForm(r, 0, e)))
		return
	}
	id, err := s.Store.CreateRecipe(r.Context(), rec)
	if errors.Is(err, store.ErrDuplicateSource) {
		// A double tap on Save, or the page imported in another tab: open that recipe.
		id, err = s.Store.FindRecipeBySourceURL(r.Context(), rec.SourceURL)
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	//nolint:gosec // G710: the target is /recipes/ plus an int64 id, never user text
	http.Redirect(w, r, fmt.Sprintf("/recipes/%d", id), http.StatusSeeOther)
}

func (s *server) updateRecipe(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	if !readForm(w, r) {
		return
	}
	rec, e := s.parseRecipe(r)
	rec.ID = id
	if len(e) > 0 {
		s.render(w, r, http.StatusUnprocessableEntity, views.RecipeFormPage(s.rawRecipeForm(r, id, e)))
		return
	}
	if err := s.Store.UpdateRecipe(r.Context(), rec); err != nil {
		s.fail(w, r, err)
		return
	}
	//nolint:gosec // G710: the target is /recipes/ plus an int64 id, never user text
	http.Redirect(w, r, fmt.Sprintf("/recipes/%d", id), http.StatusSeeOther)
}

func rowField(i int, name string) string { return validate.Field("ingredients", i, name) }

// parseRecipe reads the recipe form. Blank rows are skipped; errors are keyed by the row's
// position in the form, not in the stored recipe, so they show next to the right fields.
func (s *server) parseRecipe(r *http.Request) (recipes.Recipe, validate.Errors) {
	e := validate.Errors{}
	num := func(field string) int {
		n, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue(field)))
		if err != nil {
			e.Add(field, "form.not_a_number")
		}
		return n
	}
	rec := recipes.Recipe{Title: strings.TrimSpace(r.PostFormValue("title")),
		Description: strings.TrimSpace(r.PostFormValue("description")),
		Lang:        s.Catalog.Locale(), Source: "manual",
		Servings: num("servings"), TotalMinutes: num("total_minutes"),
		ActiveMinutes: num("active_minutes"),
		Tags:          recipes.SplitTags(r.PostFormValue("tags")),
		Steps:         recipes.SplitLines(r.PostFormValue("steps")),
		Diets:         uniqueOptions(r.PostForm["diets"]),
		Allergens:     uniqueOptions(r.PostForm["allergens"])}
	if src := strings.TrimSpace(r.PostFormValue("source_url")); src != "" {
		rec.Source, rec.SourceURL = "imported", src
	}
	var raw []int // raw[k] is the form row of rec.Ingredients[k]
	for i := range _rowsMax {
		name := strings.TrimSpace(r.PostFormValue(rowField(i, "name")))
		qty := strings.TrimSpace(r.PostFormValue(rowField(i, "quantity")))
		if name == "" && qty == "" {
			continue
		}
		q, err := recipes.ParseQuantity(qty)
		if err != nil {
			e.Add(rowField(i, "quantity"), "form.not_a_number")
		}
		raw = append(raw, i)
		rec.Ingredients = append(rec.Ingredients, recipes.Ingredient{Name: name, Quantity: q,
			Unit: r.PostFormValue(rowField(i, "unit")), Section: r.PostFormValue(rowField(i, "section")),
			Optional: r.PostFormValue(rowField(i, "optional")) == "on"})
	}
	for k, v := range rec.Validate() {
		var i int
		var field string
		if n, _ := fmt.Sscanf(k, "ingredients.%d.%s", &i, &field); n == 2 && i < len(raw) {
			e.Add(rowField(raw[i], field), v)
			continue
		}
		e.Add(k, v)
	}
	return rec, e
}

func uniqueOptions(vs []string) []string {
	return household.Member{Diets: vs}.Clean().Diets
}

// recipeForm fills the form from a stored (or empty) recipe, plus blank rows.
func (s *server) recipeForm(rec recipes.Recipe, e validate.Errors) views.RecipeForm {
	f := s.formOptions(views.RecipeForm{ID: rec.ID, Title: rec.Title,
		Description: rec.Description, Servings: strconv.Itoa(rec.Servings),
		TotalMinutes: strconv.Itoa(rec.TotalMinutes), ActiveMinutes: strconv.Itoa(rec.ActiveMinutes),
		Tags: strings.Join(rec.Tags, ", "), Steps: strings.Join(rec.Steps, "\n"), Errors: e,
		SourceURL: rec.SourceURL},
		rec.Diets, rec.Allergens)
	for i, in := range rec.Ingredients {
		q := ""
		if in.Quantity > 0 {
			q = s.Catalog.Quantity(in.Quantity)
		}
		f.Rows = append(f.Rows, views.IngredientRow{Index: i, Name: in.Name, Quantity: q,
			Unit: in.Unit, Section: in.Section, Optional: in.Optional})
	}
	return withBlankRows(f, len(rec.Ingredients))
}

// rawRecipeForm re-displays exactly what was posted, rows at their original positions.
func (s *server) rawRecipeForm(r *http.Request, id int64, e validate.Errors) views.RecipeForm {
	f := s.formOptions(views.RecipeForm{ID: id, Title: r.PostFormValue("title"),
		Description: r.PostFormValue("description"), Servings: r.PostFormValue("servings"),
		TotalMinutes: r.PostFormValue("total_minutes"), ActiveMinutes: r.PostFormValue("active_minutes"),
		Tags: r.PostFormValue("tags"), Steps: r.PostFormValue("steps"), Errors: e,
		SourceURL: r.PostFormValue("source_url")},
		r.PostForm["diets"], r.PostForm["allergens"])
	last := -1
	for i := range _rowsMax {
		if r.PostFormValue(rowField(i, "name")) != "" || r.PostFormValue(rowField(i, "quantity")) != "" {
			last = i
		}
	}
	for i := 0; i <= last; i++ {
		f.Rows = append(f.Rows, views.IngredientRow{Index: i,
			Name: r.PostFormValue(rowField(i, "name")), Quantity: r.PostFormValue(rowField(i, "quantity")),
			Unit: r.PostFormValue(rowField(i, "unit")), Section: r.PostFormValue(rowField(i, "section")),
			Optional: r.PostFormValue(rowField(i, "optional")) == "on"})
	}
	return withBlankRows(f, last+1)
}

func withBlankRows(f views.RecipeForm, from int) views.RecipeForm {
	for i := from; i < min(from+_blankRows, _rowsMax); i++ {
		f.Rows = append(f.Rows, views.IngredientRow{Index: i, Section: "produce"})
	}
	return f
}

func (s *server) formOptions(f views.RecipeForm, diets, allergens []string) views.RecipeForm {
	c := s.Catalog
	f.Diets = options(household.Diets, diets, c.Diet)
	f.Allergens = options(household.Allergens, allergens, c.Allergen)
	f.Units = append([]views.Option{{Value: "", Label: c.T("recipe.no_unit")}},
		options(recipes.Units, nil, c.Unit)...)
	f.Sections = options(recipes.Sections, nil, c.Section)
	return f
}
