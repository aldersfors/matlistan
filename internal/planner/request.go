// Package planner asks a language model for a week of dinners and checks the answer against
// the household's rules before anything is stored.
package planner

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/weekplan"
)

// Member is a household member as the model sees it: no name.
type Member struct {
	Age       int      `json:"age"`
	Diets     []string `json:"diets,omitempty"`
	Allergens []string `json:"allergens,omitempty"`
	Likes     string   `json:"likes,omitempty"`
	Dislikes  string   `json:"dislikes,omitempty"`
}

// Candidate is a library recipe the model may choose.
type Candidate struct {
	ID               int64    `json:"id"`
	Title            string   `json:"title"`
	TotalMinutes     int      `json:"total_minutes"`
	Tags             []string `json:"tags,omitempty"`
	Diets            []string `json:"diets,omitempty"`
	Allergens        []string `json:"allergens,omitempty"`
	WeeksSinceCooked int      `json:"weeks_since_cooked,omitempty"`
	Rating           float64  `json:"rating,omitempty"`
	Ratings          int      `json:"ratings,omitempty"`
}

// Request is everything one planning call knows.
type Request struct {
	Key        weekplan.Key
	Locale     i18n.Locale
	Month      time.Month
	Days       [7]weekplan.DaySpec
	Members    []Member
	Allergens  []string
	Diets      []string
	Settings   household.Settings
	Recent     []string
	Candidates []Candidate
	Only       map[int]bool // the days to plan; nil plans every planned day
	Keep       []string     // dinners that stay this week
	UseUp      []string
}

// plans reports whether this call picks a dinner for d.
func (r Request) plans(d weekplan.DaySpec) bool {
	return d.Planned && (r.Only == nil || r.Only[d.Day])
}

// Proposal is the model's answer, shaped by Schema.
type Proposal struct {
	Days []ProposedDay `json:"days"`
}

// ProposedDay is one dinner: a library recipe or a new one, with a reason.
type ProposedDay struct {
	Day             int        `json:"day"`
	Why             string     `json:"why"`
	LibraryRecipeID *int64     `json:"library_recipe_id"`
	NewRecipe       *NewRecipe `json:"new_recipe"`
}

// NewRecipe is a recipe the model wrote.
type NewRecipe struct {
	Title         string          `json:"title"`
	Description   string          `json:"description"`
	Servings      int             `json:"servings"`
	ActiveMinutes int             `json:"active_minutes"`
	TotalMinutes  int             `json:"total_minutes"`
	Tags          []string        `json:"tags"`
	Steps         []string        `json:"steps"`
	Diets         []string        `json:"diets"`
	Allergens     []string        `json:"allergens"`
	Ingredients   []NewIngredient `json:"ingredients"`
}

// NewIngredient is one line of a NewRecipe.
type NewIngredient struct {
	Name     string  `json:"name"`
	Quantity float64 `json:"quantity"`
	Unit     string  `json:"unit"`
	Section  string  `json:"section"`
	Optional bool    `json:"optional"`
}

var errNotJSON = errors.New("the answer is not the requested JSON object")

// ParseProposal reads the model's JSON answer.
func ParseProposal(text string) (Proposal, error) {
	var p Proposal
	d := json.NewDecoder(strings.NewReader(strings.TrimSpace(text)))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return Proposal{}, fmt.Errorf("%w: %w", errNotJSON, err)
	}
	return p, nil
}
