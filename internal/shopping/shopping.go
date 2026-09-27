// Package shopping turns a week's dinners into a shopping list.
package shopping

import (
	"cmp"
	"math"
	"slices"
	"strings"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/weekplan"
)

// Use is one ingredient of one planned dinner.
type Use struct {
	Day, Servings, RecipeServings int
	Ingredient                    recipes.Ingredient
}

// Item is one line of the list.
type Item struct {
	ID                        int64
	Name, Section, Unit       string
	Quantity                  float64
	Days                      []int
	Optional, Manual, Checked bool
}

// List is a week's shopping list.
type List struct {
	ID       int64
	Key      weekplan.Key
	Items    []Item
	Excluded int
}

type dimension int

const (
	dimNone dimension = iota // to taste
	dimMass
	dimVolume
	dimCount
)

// _base is each unit's dimension and size in the dimension's base unit (g, ml, pcs).
var _base = map[string]struct {
	dim  dimension
	size float64
}{
	"g": {dimMass, 1}, "kg": {dimMass, 1000},
	"ml": {dimVolume, 1}, "dl": {dimVolume, 100}, "l": {dimVolume, 1000},
	"tbsp": {dimVolume, 15}, "tsp": {dimVolume, 5}, "pinch": {dimVolume, 1},
	"pcs": {dimCount, 1},
}

type group struct {
	name, section string
	dim           dimension
	units         map[string]bool
	base          float64 // sum in the base unit
	sameUnit      float64 // sum in the single unit, when there is one
	days          []int
	optional      bool
}

// Build scales every use to its day's servings, merges equal ingredients per dimension,
// leaves out staples and orders the list by store section, then name.
func Build(uses []Use, staples []household.Staple) ([]Item, int) {
	isStaple := map[string]bool{}
	for _, s := range staples {
		isStaple[household.StapleKey(s.Name)] = true
	}
	groups := map[string]*group{}
	measured := map[string]bool{} // names with an amount in some dimension
	excluded := map[string]bool{}
	var order []string
	for _, u := range uses {
		in := u.Ingredient
		name := household.StapleKey(in.Name)
		if isStaple[name] {
			excluded[name] = true
			continue
		}
		dim := dimNone
		if in.Quantity > 0 {
			dim = _base[in.Unit].dim
			measured[name] = true
		}
		key := name + "\x00" + string(rune('0'+dim))
		g, ok := groups[key]
		if !ok {
			g = &group{name: strings.Join(strings.Fields(in.Name), " "), section: in.Section,
				dim: dim, units: map[string]bool{}, optional: true}
			groups[key] = g
			order = append(order, key)
		}
		q := in.Quantity * float64(u.Servings) / float64(max(1, u.RecipeServings))
		if dim != dimNone {
			g.units[in.Unit] = true
			g.base += q * _base[in.Unit].size
			g.sameUnit += q
		}
		if !slices.Contains(g.days, u.Day) {
			g.days = append(g.days, u.Day)
		}
		g.optional = g.optional && in.Optional
	}
	// A "to taste" use of an ingredient that also has an amount is dropped, but its days and
	// its need (not optional) carry over to the amounts, so the list still says who needs it.
	for _, key := range order {
		g := groups[key]
		name := household.StapleKey(g.name)
		if g.dim != dimNone || !measured[name] {
			continue
		}
		for _, other := range order {
			o := groups[other]
			if o.dim == dimNone || household.StapleKey(o.name) != name {
				continue
			}
			for _, d := range g.days {
				if !slices.Contains(o.days, d) {
					o.days = append(o.days, d)
				}
			}
			o.optional = o.optional && g.optional
		}
	}
	var items []Item
	for _, key := range order {
		g := groups[key]
		if g.dim == dimNone && measured[household.StapleKey(g.name)] {
			continue
		}
		it := Item{Name: g.name, Section: g.section, Optional: g.optional,
			Days: slices.Sorted(slices.Values(g.days))}
		it.Quantity, it.Unit = amount(g)
		items = append(items, it)
	}
	slices.SortStableFunc(items, func(a, b Item) int {
		return cmp.Or(cmp.Compare(slices.Index(recipes.Sections, a.Section),
			slices.Index(recipes.Sections, b.Section)),
			strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)))
	})
	return items, len(excluded)
}

// amount picks the unit to show: the single unit used, or a sensible one for the sum.
func amount(g *group) (float64, string) {
	if g.dim == dimNone {
		return 0, ""
	}
	if len(g.units) == 1 {
		for u := range g.units {
			if u == "pcs" {
				return math.Ceil(g.sameUnit - 1e-9), u
			}
			return round2(g.sameUnit), u
		}
	}
	switch g.dim {
	case dimMass:
		if g.base >= 1000 {
			return round2(g.base / 1000), "kg"
		}
		return math.Round(g.base), "g"
	case dimVolume:
		switch {
		case g.base >= 1000:
			return round2(g.base / 1000), "l"
		case g.base >= 100:
			return round2(g.base / 100), "dl"
		}
		return math.Round(g.base), "ml"
	}
	return math.Ceil(g.base - 1e-9), "pcs"
}

func round2(q float64) float64 { return math.Round(q*100) / 100 }
