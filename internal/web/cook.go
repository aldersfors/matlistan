package web

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"

	"github.com/jalet/matlistan/internal/web/views"
)

var _minutes = regexp.MustCompile(`(?i)\b(\d{1,3})\s*(?:minuter|minutes|min)\b`)

// minutesIn finds "20 minuter", "10 min" or "35 minutes" in a step; 0 when there is none
// or the step says "1 minut" (too short to need a timer).
func minutesIn(step string) int {
	m := _minutes.FindStringSubmatch(step)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	if n < 2 {
		return 0
	}
	return n
}

// servingsParam reads ?servings=, falling back to base outside 1..20.
func servingsParam(r *http.Request, base int) int {
	n, err := strconv.Atoi(r.URL.Query().Get("servings"))
	if err != nil || n < 1 || n > 20 {
		return base
	}
	return n
}

// dayParam reads ?day= (1..7); 0 when absent or invalid.
func dayParam(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("day"))
	if err != nil || n < 1 || n > 7 {
		return 0
	}
	return n
}

func cookHref(id int64, servings, day int) string {
	href := fmt.Sprintf("/recipes/%d/cook?servings=%d", id, servings)
	if day > 0 {
		href += fmt.Sprintf("&day=%d", day)
	}
	return href
}

func (s *server) cook(w http.ResponseWriter, r *http.Request) {
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
	v := views.Cook{Title: rec.Title, Meta: c.N("recipes.servings", servings),
		Progress: c.T("cook.progress"), TimerLine: c.T("cook.timer_line"), DayClass: "day-1"}
	if day > 0 {
		v.DayClass = fmt.Sprintf("day-%d", day)
	}
	for _, in := range rec.Scaled(servings) {
		v.Ingredients = append(v.Ingredients, s.ingredientLine(in))
	}
	for _, st := range rec.Steps {
		step := views.CookStep{Text: st, Minutes: minutesIn(st), DoneLabel: c.T("cook.timer_done")}
		if step.Minutes > 0 {
			step.TimerLabel = c.N("cook.timer", step.Minutes)
		}
		v.Steps = append(v.Steps, step)
	}
	s.render(w, r, http.StatusOK, views.CookPage(v))
}
