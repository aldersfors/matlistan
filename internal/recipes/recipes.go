// Package recipes models the recipe library.
package recipes

import (
	"errors"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/validate"
)

// Units are the metric units an ingredient may use; pcs counts whole items.
var Units = []string{"g", "kg", "ml", "dl", "l", "pcs", "tbsp", "tsp", "pinch"}

// Sections are the store sections the shopping list groups by, in walking order.
var Sections = []string{"produce", "dairy", "meat_fish", "pantry", "frozen", "bakery", "other"}

// Sources say where a recipe came from.
var Sources = []string{"manual", "generated", "imported"}

// Limits on one recipe.
const (
	IngredientsMax = 60
	StepsMax       = 40
	TagsMax        = 10
	quantityMax    = 100000
)

// Ingredient is one line of a recipe. Quantity 0 means "to taste" and takes no unit.
type Ingredient struct {
	Name          string
	Quantity      float64
	Unit, Section string
	Optional      bool
}

// Recipe is a dish in the library.
type Recipe struct {
	ID                                    int64
	Title, Description                    string
	Lang                                  i18n.Locale
	Servings, ActiveMinutes, TotalMinutes int
	Tags, Steps, Diets, Allergens         []string
	Source                                string
	SourceURL                             string // https page it was imported from; "" otherwise
	Ingredients                           []Ingredient
	Rating                                Rating
	Key                                   string // stable identity, also used in git
	Managed                               string // "", "inline" (recipes:) or "url" (recipeURLs)
}

// Summary is a recipe as the library list shows it.
type Summary struct {
	ID           int64
	Title        string
	TotalMinutes int
	Tags         []string
	Rating       Rating
}

// Validate checks a recipe as entered or generated. Ingredient errors are keyed by index:
// "ingredients.2.unit".
func (r Recipe) Validate() validate.Errors {
	e := validate.Errors{}
	e.Text("title", r.Title, 1, 120)
	e.Text("description", r.Description, 0, 1000)
	e.Range("servings", r.Servings, 1, 20)
	e.Range("total_minutes", r.TotalMinutes, 1, 1440)
	e.Range("active_minutes", r.ActiveMinutes, 0, r.TotalMinutes)
	if !slices.Contains(i18n.Supported, r.Lang) {
		e.Add("lang", "form.unknown_option")
	}
	e.Options("source", []string{r.Source}, Sources)
	switch {
	case r.SourceURL != "" && r.Source != "imported", r.SourceURL == "" && r.Source == "imported":
		e.Add("source", "form.unknown_option")
	}
	if r.SourceURL != "" {
		if n, err := NormalizeSourceURL(r.SourceURL); err != nil || n != r.SourceURL {
			e.Add("source_url", "form.invalid_url")
		}
	}
	e.Options("diets", r.Diets, household.Diets)
	e.Options("allergens", r.Allergens, household.Allergens)
	if len(r.Tags) > TagsMax {
		e.Add("tags", "form.too_long")
	}
	for _, t := range r.Tags {
		e.Text("tags", t, 1, 30)
	}
	switch {
	case len(r.Ingredients) == 0:
		e.Add("ingredients", "form.required")
	case len(r.Ingredients) > IngredientsMax:
		e.Add("ingredients", "form.too_long")
	}
	for i, in := range r.Ingredients {
		f := func(name string) string { return validate.Field("ingredients", i, name) }
		e.Text(f("name"), in.Name, 1, 80)
		switch {
		case in.Quantity < 0 || in.Quantity > quantityMax:
			e.Add(f("quantity"), "form.out_of_range")
		case in.Quantity > 0 && in.Unit == "":
			e.Add(f("unit"), "form.unit_required")
		}
		if in.Unit != "" {
			e.Options(f("unit"), []string{in.Unit}, Units)
		}
		e.Options(f("section"), []string{in.Section}, Sections)
	}
	switch {
	case len(r.Steps) == 0:
		e.Add("steps", "form.required")
	case len(r.Steps) > StepsMax:
		e.Add("steps", "form.too_long")
	}
	for _, s := range r.Steps {
		e.Text("steps", s, 1, 1000)
	}
	return e
}

// Scaled returns the ingredients for servings portions. To-taste amounts stay 0.
func (r Recipe) Scaled(servings int) []Ingredient {
	f := float64(servings) / float64(r.Servings)
	out := slices.Clone(r.Ingredients)
	for i := range out {
		out[i].Quantity *= f
	}
	return out
}

var errQuantity = errors.New("not a quantity")

// ParseQuantity reads an amount typed with either decimal separator; "" is 0 (to taste).
func ParseQuantity(s string) (float64, error) {
	s = strings.TrimSpace(strings.Replace(s, ",", ".", 1))
	if s == "" {
		return 0, nil
	}
	q, err := strconv.ParseFloat(s, 64)
	if err != nil || q < 0 || math.IsNaN(q) || math.IsInf(q, 0) {
		return 0, errQuantity
	}
	return q, nil
}

// SplitLines turns a textarea into trimmed, non-empty lines.
func SplitLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// SplitTags turns "a, b ,a" into sorted, lower-cased, unique tags.
func SplitTags(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			out = append(out, t)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// TitleKey is the search form of a title: trimmed and lower-cased with Unicode rules, so
// "Ärtsoppa" is found by "ärt" whatever the database collation.
func TitleKey(title string) string { return strings.ToLower(strings.TrimSpace(title)) }

// Ratings a family member can give a dinner.
const (
	ScoreLoved    = 5
	ScoreOkay     = 3
	ScoreNotAgain = 1
)

// Scores lists the ratings in the order the rating screen shows them.
var Scores = []int{ScoreLoved, ScoreOkay, ScoreNotAgain}

// Rating is the family's average score for a recipe.
type Rating struct {
	Average float64
	Count   int
}

// SourceURLMax is the longest source URL stored.
const SourceURLMax = 2000

// NormalizeSourceURL gives the form an imported recipe's link is stored and compared in:
// https only, lowercase scheme and host, no fragment, no trailing slash.
func NormalizeSourceURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Host == "" || u.User != nil {
		return "", errors.New("not an https URL")
	}
	u.Scheme, u.Host, u.Fragment, u.RawFragment = "https", strings.ToLower(u.Host), "", ""
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.RawPath = ""
	s := u.String()
	if len(s) > SourceURLMax {
		return "", errors.New("URL too long")
	}
	return s, nil
}
