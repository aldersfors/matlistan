package planner

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jalet/matlistan/internal/recipes"
	"github.com/jalet/matlistan/internal/weekplan"
)

const _whyRunesMax = 300

// Violation is one broken rule, phrased for the model.
type Violation struct {
	Day     int
	Problem string
}

func (v Violation) String() string { return fmt.Sprintf("day %d: %s", v.Day, v.Problem) }

// Feedback is the retry message listing every violation.
func Feedback(vs []Violation) string {
	lines := make([]string, len(vs))
	for i, v := range vs {
		lines[i] = "- " + v.String()
	}
	return "Your answer broke these rules. Answer again with the complete JSON object, fixing " +
		"every problem:\n" + strings.Join(lines, "\n")
}

// Resolve checks a proposal against the request and turns it into picks.
func Resolve(r Request, p Proposal, library map[int64]recipes.Recipe) ([]weekplan.Pick,
	[]Violation) {
	var vs []Violation
	add := func(day int, format string, args ...any) {
		vs = append(vs, Violation{Day: day, Problem: fmt.Sprintf(format, args...)})
	}
	planned := map[int]weekplan.DaySpec{}
	for _, d := range r.Days {
		if d.Planned && (r.SwapDay == 0 || r.SwapDay == d.Day) {
			planned[d.Day] = d
		}
	}
	candidates := map[int64]bool{}
	for _, c := range r.Candidates {
		candidates[c.ID] = true
	}
	recent := map[string]bool{}
	for _, t := range r.Recent {
		recent[recipes.TitleKey(t)] = true
	}
	taken := map[string]bool{}
	for _, t := range r.Keep {
		taken[recipes.TitleKey(t)] = true
	}
	seen := map[int]bool{}
	var picks []weekplan.Pick
	for _, pd := range p.Days {
		spec, ok := planned[pd.Day]
		switch {
		case !ok:
			add(pd.Day, "not a planned day")
			continue
		case seen[pd.Day]:
			add(pd.Day, "planned twice")
			continue
		}
		seen[pd.Day] = true
		why := strings.TrimSpace(pd.Why)
		if why == "" {
			add(pd.Day, "why is empty")
		} else if utf8.RuneCountInString(why) > _whyRunesMax {
			add(pd.Day, "why is longer than %d characters", _whyRunesMax)
		}
		rec, pick, ok := recipeFor(r, pd, library, candidates, add)
		if !ok {
			continue
		}
		pick.Day, pick.Servings, pick.Why = pd.Day, spec.Servings, why
		key := recipes.TitleKey(rec.Title)
		switch {
		case recent[key]:
			add(pd.Day, "%q was cooked recently", rec.Title)
		case taken[key]:
			add(pd.Day, "%q is already planned this week", rec.Title)
		}
		taken[key] = true
		if rec.TotalMinutes > spec.MaxMinutes {
			add(pd.Day, "takes %d minutes, the limit is %d", rec.TotalMinutes, spec.MaxMinutes)
		}
		checkRestrictions(r, pd.Day, rec, add)
		picks = append(picks, pick)
	}
	for _, d := range r.Days {
		if _, ok := planned[d.Day]; ok && !seen[d.Day] {
			add(d.Day, "no dinner planned")
		}
	}
	slices.SortFunc(vs, func(a, b Violation) int { return a.Day - b.Day })
	if len(vs) > 0 {
		return nil, vs
	}
	return picks, nil
}

// recipeFor finds or builds the day's recipe; false when the day cannot be checked further.
func recipeFor(r Request, pd ProposedDay, library map[int64]recipes.Recipe,
	candidates map[int64]bool, add func(int, string, ...any)) (recipes.Recipe, weekplan.Pick,
	bool) {
	switch {
	case (pd.LibraryRecipeID == nil) == (pd.NewRecipe == nil):
		add(pd.Day, "set exactly one of library_recipe_id and new_recipe")
		return recipes.Recipe{}, weekplan.Pick{}, false
	case pd.LibraryRecipeID != nil:
		id := *pd.LibraryRecipeID
		rec, ok := library[id]
		if !ok || !candidates[id] {
			add(pd.Day, "%d is not a candidate id", id)
			return recipes.Recipe{}, weekplan.Pick{}, false
		}
		return rec, weekplan.Pick{RecipeID: id}, true
	}
	n := pd.NewRecipe
	rec := recipes.Recipe{Title: strings.TrimSpace(n.Title), Description: n.Description,
		Lang: r.Locale, Servings: n.Servings, ActiveMinutes: n.ActiveMinutes,
		TotalMinutes: n.TotalMinutes, Tags: recipes.SplitTags(strings.Join(n.Tags, ",")),
		Steps: n.Steps, Diets: n.Diets, Allergens: n.Allergens, Source: "generated"}
	for _, in := range n.Ingredients {
		rec.Ingredients = append(rec.Ingredients, recipes.Ingredient{
			Name: strings.TrimSpace(in.Name), Quantity: in.Quantity, Unit: in.Unit,
			Section: in.Section, Optional: in.Optional})
	}
	if e := rec.Validate(); len(e) > 0 {
		for _, f := range slices.Sorted(maps.Keys(e)) {
			add(pd.Day, "%s: %s", f, e[f])
		}
		return recipes.Recipe{}, weekplan.Pick{}, false
	}
	return rec, weekplan.Pick{New: &rec}, true
}

func checkRestrictions(r Request, day int, rec recipes.Recipe, add func(int, string, ...any)) {
	for _, a := range r.Allergens {
		if slices.Contains(rec.Allergens, a) {
			add(day, "contains %s", a)
			continue
		}
		for _, in := range rec.Ingredients {
			if containsAllergen(r.Locale, a, in.Name) {
				add(day, "%q suggests %s", in.Name, a)
				break
			}
		}
	}
	for _, d := range r.Diets {
		if !suitsDiet(rec.Diets, d) {
			add(day, "does not declare %s", d)
		}
	}
	if slices.Contains(r.Diets, "no_pork") {
		for _, in := range rec.Ingredients {
			if suggestsPork(r.Locale, in.Name) {
				add(day, "%q suggests pork", in.Name)
				break
			}
		}
	}
}

// _stricter lists, for each diet, the diets that also satisfy it.
var _stricter = map[string][]string{
	"vegan":       {"vegan"},
	"vegetarian":  {"vegetarian", "vegan"},
	"pescatarian": {"pescatarian", "vegetarian", "vegan"},
	"no_pork":     {"no_pork", "pescatarian", "vegetarian", "vegan"},
}

// suitsDiet reports whether a recipe declaring these diets suits required.
func suitsDiet(declared []string, required string) bool {
	for _, d := range _stricter[required] {
		if slices.Contains(declared, d) {
			return true
		}
	}
	return false
}
