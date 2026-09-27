package planner

import (
	"encoding/json"

	"github.com/aldersfors/matlistan/internal/i18n"
)

var _languages = map[i18n.Locale]string{i18n.EN: "English", i18n.SV: "Swedish"}

// SystemPrompt is the fixed part of every call; it never changes between calls with the
// same locale, so the API can cache it.
func SystemPrompt(l i18n.Locale) string {
	return `You plan family dinners for one household for one week.

You receive a JSON document describing the week. Answer with the JSON object the response
schema defines and nothing else.

Rules. Every rule is checked by a program; an answer that breaks one is rejected.
- Give exactly one entry for every day with "planned": true, and none for other days.
- For each entry choose either a library recipe (set "library_recipe_id" to a candidate id and
  "new_recipe" to null) or write a new recipe (set "library_recipe_id" to null).
- Take about "library_share" percent of the dinners from the candidates. Prefer candidates with
  a high "weeks_since_cooked" or none.
- "rating" is the family's average score from 1 to 5 over "ratings" votes. Prefer dinners the
  family rated highly, and when that is why you chose one, say so in "why".
- Never plan a dish listed in "recent", and never the same dish twice in one week.
- No dinner may contain any allergen listed in "allergens". List every allergen a new recipe
  contains in its "allergens" field, using the allowed values.
- Every dinner must suit every diet listed in "diets". List the diets a new recipe suits.
- A dinner's total time must not exceed the day's "max_minutes".
- Write new recipes for the day's "servings" portions; set "servings" to that number.
- Use metric amounts. Ingredient names are short, lower case and in the output language, for
  example "yellow onion". Use quantity 0 and unit "" for "to taste". Every quantity above 0
  needs a unit.
- Prefer seasonal produce for the given month and simple weeknight cooking.
- "why" is one short, friendly sentence telling the family why this dinner was chosen, using
  the week's facts: history, season, the day's conditions, likes and dislikes. Never mention
  allergies by name and never address a family member by name.

Write every human-readable text (titles, descriptions, steps, ingredient names, why) in ` +
		_languages[l] + `.`
}

type promptDay struct {
	Day        int    `json:"day"`
	Weekday    string `json:"weekday"`
	Planned    bool   `json:"planned"`
	QuickNight bool   `json:"quick_evening,omitempty"`
	Guests     int    `json:"guests,omitempty"`
	Servings   int    `json:"servings"`
	MaxMinutes int    `json:"max_minutes"`
}

type promptDoc struct {
	Week         string      `json:"week"`
	Month        string      `json:"month"`
	Household    []Member    `json:"household"`
	Allergens    []string    `json:"allergens"`
	Diets        []string    `json:"diets"`
	LibraryShare int         `json:"library_share"`
	Days         []promptDay `json:"days"`
	Recent       []string    `json:"recent"`
	Candidates   []Candidate `json:"candidates"`
	Task         string      `json:"task"`
	KeepOtherDay []string    `json:"other_days_this_week,omitempty"`
}

// UserPrompt renders the week as JSON for the model.
func UserPrompt(r Request) (string, error) {
	doc := promptDoc{Week: r.Key.String(), Month: r.Month.String(), Household: r.Members,
		Allergens: orEmpty(r.Allergens), Diets: orEmpty(r.Diets),
		LibraryShare: r.Settings.LibraryShare, Recent: orEmpty(r.Recent),
		Candidates: r.Candidates, Task: "Plan every planned day of the week."}
	if doc.Candidates == nil {
		doc.Candidates = []Candidate{}
	}
	for _, d := range r.Days {
		planned := d.Planned && (r.SwapDay == 0 || r.SwapDay == d.Day)
		doc.Days = append(doc.Days, promptDay{Day: d.Day, Weekday: d.Date.Weekday().String(),
			Planned: planned, QuickNight: d.Busy, Guests: d.Guests, Servings: d.Servings,
			MaxMinutes: d.MaxMinutes})
	}
	if r.SwapDay != 0 {
		doc.Task = "Replace the dinner of the only planned day with a different one. It must " +
			"differ from the other days' dinners and should not share their main ingredient."
		doc.KeepOtherDay = orEmpty(r.Keep)
	}
	b, err := json.Marshal(doc)
	return string(b), err
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
