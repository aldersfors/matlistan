// Package household models the family: who eats, what they cannot or will not eat, and how
// the week is planned.
package household

import (
	"slices"
	"strings"
	"time"

	"github.com/aldersfors/matlistan/internal/validate"
)

// Allergens are the 14 EU labelling allergens (Regulation (EU) No 1169/2011, Annex II).
var Allergens = []string{"gluten", "crustaceans", "eggs", "fish", "peanuts", "soybeans", "milk",
	"nuts", "celery", "mustard", "sesame", "sulphites", "lupin", "molluscs"}

// Diets are the eating patterns the planner respects.
var Diets = []string{"vegetarian", "vegan", "pescatarian", "no_pork"}

// Member is one person in the household.
type Member struct {
	ID               int64
	Name             string
	BirthYear        int
	Subject          string // OIDC subject of the linked login, "" when none
	Diets, Allergens []string
	Likes, Dislikes  string
}

// Clean trims text and sorts and de-duplicates choices. Forms call it before Validate.
func (m Member) Clean() Member {
	m.Name = strings.TrimSpace(m.Name)
	m.Likes = strings.TrimSpace(m.Likes)
	m.Dislikes = strings.TrimSpace(m.Dislikes)
	m.Diets = uniq(m.Diets)
	m.Allergens = uniq(m.Allergens)
	return m
}

func uniq(vs []string) []string {
	out := slices.Clone(vs)
	if out == nil {
		out = []string{}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Validate checks a member as entered; now bounds the birth year to the last 120 years.
func (m Member) Validate(now time.Time) validate.Errors {
	e := validate.Errors{}
	e.Text("name", m.Name, 1, 60)
	e.Range("birth_year", m.BirthYear, now.Year()-120, now.Year())
	e.Options("diets", m.Diets, Diets)
	e.Options("allergens", m.Allergens, Allergens)
	e.Text("likes", m.Likes, 0, 500)
	e.Text("dislikes", m.Dislikes, 0, 500)
	return e
}

// Age in whole years by calendar year; never negative.
func Age(birthYear int, now time.Time) int { return max(0, now.Year()-birthYear) }

// PortionFactor is the share of an adult portion a person of this age eats. It is a
// planning heuristic, not nutritional advice.
func PortionFactor(age int) float64 {
	switch {
	case age <= 3:
		return 0.3
	case age <= 8:
		return 0.5
	case age <= 12:
		return 0.75
	default:
		return 1
	}
}

// Settings are the household's planning rules.
type Settings struct {
	DinnersPerWeek    int
	WeeknightMinutes  int
	RepeatWindowWeeks int
	LibraryShare      int // percent of dinners taken from the recipe library
}

// DefaultSettings match the database defaults.
func DefaultSettings() Settings {
	return Settings{DinnersPerWeek: 7, WeeknightMinutes: 45, RepeatWindowWeeks: 6,
		LibraryShare: 50}
}

// Validate checks every setting against its allowed range.
func (s Settings) Validate() validate.Errors {
	e := validate.Errors{}
	e.Range("dinners_per_week", s.DinnersPerWeek, 1, 7)
	e.Range("weeknight_minutes", s.WeeknightMinutes, 10, 240)
	e.Range("repeat_window_weeks", s.RepeatWindowWeeks, 1, 26)
	e.Range("library_share", s.LibraryShare, 0, 100)
	return e
}

// Staple is an item that is always at home and never goes on the shopping list.
type Staple struct {
	ID   int64
	Name string
}

// StapleName collapses runs of whitespace: "  Havre   gryn " is "Havre gryn".
func StapleName(s string) string { return strings.Join(strings.Fields(s), " ") }

// StapleKey is how staples are compared: StapleName, lower-cased.
func StapleKey(s string) string { return strings.ToLower(StapleName(s)) }
