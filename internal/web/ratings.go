package web

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"

	"github.com/jalet/matlistan/internal/household"
	"github.com/jalet/matlistan/internal/recipes"
	"github.com/jalet/matlistan/internal/store"
	"github.com/jalet/matlistan/internal/web/views"
	"github.com/jalet/matlistan/internal/weekplan"
)

var _scoreKeys = map[int]string{recipes.ScoreLoved: "rate.loved", recipes.ScoreOkay: "rate.okay",
	recipes.ScoreNotAgain: "rate.not_again"}

func (s *server) rateWeek(w http.ResponseWriter, r *http.Request) {
	k, ok := s.weekKey(r)
	if !ok {
		s.badRequest(w, r)
		return
	}
	plan, err := s.Store.GetPlan(r.Context(), k)
	if err != nil || plan.Status != weekplan.StatusApproved {
		if err == nil || errors.Is(err, store.ErrNotFound) {
			s.notFound(w, r)
			return
		}
		s.fail(w, r, err)
		return
	}
	members, err := s.Store.ListMembers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	scores, err := s.Store.WeekRatings(r.Context(), k)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	c := s.Catalog
	monday := k.Monday(s.Now().Location())
	v := views.RateWeek{Label: c.WeekLabel(k.Week), Hint: c.T("rate.hint"),
		Range: c.T("week.range", "from", c.Date(monday), "to", c.Date(monday.AddDate(0, 0, 6)))}
	for _, e := range plan.Entries {
		d := views.RateDinner{Day: e.Day, Title: e.Title,
			DayName: c.WeekdayShort(monday.AddDate(0, 0, e.Day-1).Weekday())}
		for _, m := range members {
			d.Rows = append(d.Rows, s.rateRow(k, e.Day, m, scores[e.Day][m.ID]))
		}
		v.Dinners = append(v.Dinners, d)
	}
	s.render(w, r, http.StatusOK, views.RateWeekPage(v))
}

func (s *server) rateRow(k weekplan.Key, day int, m household.Member, score int) views.RateRow {
	row := views.RateRow{Year: k.Year, Week: k.Week, Day: day, MemberID: m.ID, Name: m.Name,
		Score: score}
	for _, sc := range recipes.Scores {
		label := s.Catalog.T(_scoreKeys[sc])
		row.Choices = append(row.Choices, views.RateChoice{Score: sc, Pressed: sc == score,
			Label: label, AriaLabel: s.Catalog.T("rate.for", "name", m.Name, "choice", label)})
	}
	return row
}

func (s *server) rate(w http.ResponseWriter, r *http.Request) {
	if !readForm(w, r) {
		return
	}
	k, ok := s.weekKey(r)
	day, err1 := strconv.Atoi(r.PostFormValue("day"))
	member, err2 := strconv.ParseInt(r.PostFormValue("member"), 10, 64)
	score, err3 := strconv.Atoi(r.PostFormValue("score"))
	if !ok || err1 != nil || err2 != nil || err3 != nil || day < 1 || day > 7 ||
		!slices.Contains(recipes.Scores, score) {
		s.badRequest(w, r)
		return
	}
	if err := s.Store.SetRating(r.Context(), k, day, member, score); err != nil {
		s.fail(w, r, err)
		return
	}
	if r.Header.Get("HX-Request") != "true" {
		http.Redirect(w, r, fmt.Sprintf("/week/rate?y=%d&w=%d", k.Year, k.Week),
			http.StatusSeeOther)
		return
	}
	m, err := s.Store.GetMember(r.Context(), member)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.RateRowView(s.rateRow(k, day, m, score)))
}
