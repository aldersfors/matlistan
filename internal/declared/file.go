// Package declared reads the household declared in git: members, recipes and recipe links
// from the chart's household file, and the YAML the app offers for copying.
package declared

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/keys"
	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/validate"
)

// The household file's shape. JSON tags because sigs.k8s.io/yaml goes through JSON.
type file struct {
	Members    []fileMember `json:"members"`
	Recipes    []fileRecipe `json:"recipes"`
	RecipeURLs []string     `json:"recipeURLs"`
}

type fileMember struct {
	Key       string   `json:"key"`
	Name      string   `json:"name"`
	BirthYear int      `json:"birthYear"`
	Diets     []string `json:"diets,omitempty"`
	Allergens []string `json:"allergens,omitempty"`
	Likes     string   `json:"likes,omitempty"`
	Dislikes  string   `json:"dislikes,omitempty"`
}

type fileRecipe struct {
	Key           string           `json:"key"`
	Title         string           `json:"title"`
	Lang          string           `json:"lang,omitempty"`
	Servings      int              `json:"servings"`
	ActiveMinutes int              `json:"activeMinutes,omitempty"`
	TotalMinutes  int              `json:"totalMinutes"`
	Description   string           `json:"description,omitempty"`
	Tags          []string         `json:"tags,omitempty"`
	Diets         []string         `json:"diets,omitempty"`
	Allergens     []string         `json:"allergens,omitempty"`
	SourceURL     string           `json:"sourceURL,omitempty"`
	Steps         []string         `json:"steps"`
	Ingredients   []fileIngredient `json:"ingredients"`
}

type fileIngredient struct {
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity,omitempty"`
	Unit     string  `json:"unit,omitempty"`
	Section  string  `json:"section"`
	Optional bool    `json:"optional,omitempty"`
}

// Household is the parsed, validated file, in the app's own types.
type Household struct {
	Members    []household.Member
	Recipes    []recipes.Recipe
	RecipeURLs []string // normalised
}

// Parse reads and validates the household file, reporting every problem at once. msg turns
// a validation key ("form.required") into readable text; lang is the default for recipes
// without one; now is for birth-year checks.
func Parse(data []byte, lang i18n.Locale, now time.Time, msg func(string) string) (Household,
	error) {
	var f file
	if err := yaml.UnmarshalStrict(data, &f); err != nil {
		return Household{}, fmt.Errorf("household.yaml: %w", err)
	}
	var probs []string
	add := func(where, key string) { probs = append(probs, where+": "+key) }
	addAll := func(where string, e validate.Errors) {
		for field, key := range e {
			add(where+"."+field, msg(key))
		}
	}
	var h Household
	seen := map[string]bool{}
	for _, fm := range f.Members {
		where := "members[" + fm.Key + "]"
		if !keys.Pattern.MatchString(fm.Key) {
			add(where+".key", "must match "+keys.Pattern.String())
		}
		if seen[fm.Key] {
			add(where, "key used twice")
		}
		seen[fm.Key] = true
		m := household.Member{Key: fm.Key, Managed: "inline", Name: fm.Name, BirthYear: fm.BirthYear,
			Diets: fm.Diets, Allergens: fm.Allergens, Likes: fm.Likes, Dislikes: fm.Dislikes}.Clean()
		addAll(where, m.Validate(now))
		h.Members = append(h.Members, m)
	}
	seen = map[string]bool{}
	sources := map[string]bool{}
	for _, fr := range f.Recipes {
		where := "recipes[" + fr.Key + "]"
		if !keys.Pattern.MatchString(fr.Key) {
			add(where+".key", "must match "+keys.Pattern.String())
		}
		if seen[fr.Key] {
			add(where, "key used twice")
		}
		seen[fr.Key] = true
		l := lang
		if fr.Lang != "" {
			var err error
			if l, err = i18n.ParseLocale(fr.Lang); err != nil {
				add(where+".lang", err.Error())
			}
		}
		r := recipes.Recipe{Key: fr.Key, Managed: "inline", Source: "manual", Title: fr.Title,
			Lang: l, Servings: fr.Servings, ActiveMinutes: fr.ActiveMinutes,
			TotalMinutes: fr.TotalMinutes, Description: fr.Description, Tags: fr.Tags,
			Diets: fr.Diets, Allergens: fr.Allergens, Steps: fr.Steps}
		for _, in := range fr.Ingredients {
			r.Ingredients = append(r.Ingredients, recipes.Ingredient{Name: in.Name,
				Quantity: in.Quantity, Unit: in.Unit, Section: in.Section, Optional: in.Optional})
		}
		if fr.SourceURL != "" {
			n, err := recipes.NormalizeSourceURL(fr.SourceURL)
			if err != nil {
				add(where+".sourceURL", err.Error())
			}
			// The app's rule: a recipe with a source link is an imported one.
			r.SourceURL, r.Source = n, "imported"
			sources[n] = true
		}
		addAll(where, r.Validate())
		h.Recipes = append(h.Recipes, r)
	}
	links := map[string]bool{}
	for i, raw := range f.RecipeURLs {
		where := "recipeURLs[" + strconv.Itoa(i) + "]"
		n, err := recipes.NormalizeSourceURL(raw)
		switch {
		case err != nil:
			add(where, err.Error())
			continue
		case links[n]:
			add(where, "listed twice")
			continue
		case sources[n]:
			add(where, "also a recipe's sourceURL")
			continue
		}
		links[n] = true
		h.RecipeURLs = append(h.RecipeURLs, n)
	}
	if len(probs) > 0 {
		return Household{}, errors.New("household.yaml:\n  " + strings.Join(probs, "\n  "))
	}
	return h, nil
}
