package importer

import (
	"context"
	"errors"
	"strings"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/recipes"
)

// ErrNoRecipe means the page has no recipe the importer can read.
var ErrNoRecipe = errors.New("no recipe found on the page")

// Importer reads a recipe page into an unsaved recipe. Norm may be nil when no model is
// configured; then only JSON-LD pages work, with ingredients left as written.
type Importer struct {
	Fetch Fetcher
	Norm  Normaliser
	Lang  i18n.Locale
}

// Import fetches rawURL and returns a draft recipe plus note keys for the form.
func (im Importer) Import(ctx context.Context, rawURL string) (recipes.Recipe, []string, error) {
	p, err := im.Fetch.Fetch(ctx, rawURL)
	if err != nil {
		return recipes.Recipe{}, nil, err
	}
	src, err := recipes.NormalizeSourceURL(p.URL)
	if err != nil {
		return recipes.Recipe{}, nil, ErrInvalidURL
	}
	var notes []string
	d, ok := extractRecipe(p.HTML)
	if !ok {
		if im.Norm == nil {
			return recipes.Recipe{}, nil, ErrNoRecipe
		}
		text, _ := pageText(p.HTML)
		if d, err = im.Norm.FromText(ctx, text, im.Lang); err != nil {
			return recipes.Recipe{}, nil, err
		}
		if d.Title == "" || len(d.IngredientLines) == 0 {
			return recipes.Recipe{}, nil, ErrNoRecipe
		}
	} else if im.Norm != nil && foreign(d.Lang, im.Lang) {
		if t, terr := im.Norm.Translate(ctx, d, im.Lang); terr == nil {
			d = t
			notes = append(notes, "import.note.translated")
		}
	}
	r := recipes.Recipe{Title: d.Title, Description: d.Description, Lang: im.Lang,
		Servings: d.Servings, TotalMinutes: d.TotalMinutes, ActiveMinutes: d.ActiveMinutes,
		Steps: d.Steps, Source: "imported", SourceURL: src}
	var ingr []recipes.Ingredient
	var inotes []string
	if im.Norm != nil {
		ingr, inotes, err = im.Norm.Ingredients(ctx, d.IngredientLines, im.Lang)
	}
	if im.Norm == nil || err != nil {
		ingr, inotes = rawIngredients(d.IngredientLines), []string{"import.note.no_model"}
	}
	r.Ingredients = ingr
	return r, append(notes, inotes...), nil
}

// foreign reports whether a page language (BCP 47, "" unknown) differs from the app's.
func foreign(pageLang string, app i18n.Locale) bool {
	return pageLang != "" && !strings.HasPrefix(strings.ToLower(pageLang), string(app))
}
