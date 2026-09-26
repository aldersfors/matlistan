package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/jalet/matlistan/internal/auth"
	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/store"
	"github.com/jalet/matlistan/internal/validate"
	"github.com/jalet/matlistan/internal/web/views"
	"github.com/jalet/matlistan/internal/week"
	"github.com/jalet/matlistan/internal/weekplan"
)

// weekKey reads y and w (query or form); none means the upcoming week.
func (s *server) weekKey(r *http.Request) (weekplan.Key, bool) {
	y, w := r.FormValue("y"), r.FormValue("w")
	if y == "" && w == "" {
		return weekplan.KeyOf(week.Upcoming(s.Now()).Days[0]), true
	}
	yi, err1 := strconv.Atoi(y)
	wi, err2 := strconv.Atoi(w)
	if err1 != nil || err2 != nil {
		return weekplan.Key{}, false
	}
	k, err := weekplan.NewKey(yi, wi)
	return k, err == nil
}

// _planAheadWeeks is how far ahead planning may run; with the current week it bounds what one
// person can spend on the model.
const _planAheadWeeks = 8

// plannable reports whether k is the current week or at most _planAheadWeeks after it.
func (s *server) plannable(k weekplan.Key) bool {
	now := weekplan.KeyOf(s.Now())
	return !k.Less(now) && !now.AddWeeks(_planAheadWeeks).Less(k)
}

func weekHref(k weekplan.Key) string { return fmt.Sprintf("/week?y=%d&w=%d", k.Year, k.Week) }

func (s *server) badRequest(w http.ResponseWriter, r *http.Request) {
	http.Error(w, i18n.T(r.Context(), "error.bad_request"), http.StatusBadRequest)
}

func (s *server) week(w http.ResponseWriter, r *http.Request) {
	k, ok := s.weekKey(r)
	if !ok {
		s.badRequest(w, r)
		return
	}
	v, ok := s.weekView(w, r, k, nil)
	if !ok {
		return
	}
	s.render(w, r, http.StatusOK, views.WeekPage(v))
}

// weekView loads the week; false means the error response is written.
func (s *server) weekView(w http.ResponseWriter, r *http.Request, k weekplan.Key,
	e validate.Errors) (views.Week, bool) {
	settings, err := s.Store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return views.Week{}, false
	}
	members, err := s.Store.ListMembers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return views.Week{}, false
	}
	plan, err := s.Store.GetPlan(r.Context(), k)
	switch {
	case errors.Is(err, store.ErrNotFound):
		plan = weekplan.Plan{Key: k, Status: weekplan.StatusDraft,
			Context: weekplan.DefaultContext(settings.DinnersPerWeek)}
	case err != nil:
		s.fail(w, r, err)
		return views.Week{}, false
	}
	c := s.Catalog
	monday := k.Monday(s.Now().Location())
	v := views.Week{Year: k.Year, Number: k.Week, Label: c.WeekLabel(k.Week),
		Range:      c.T("week.range", "from", c.Date(monday), "to", c.Date(monday.AddDate(0, 0, 6))),
		Configured: s.Planner != nil, Plannable: s.plannable(k), Generating: s.jobs.running(k),
		Approved: plan.Status == weekplan.StatusApproved, HasEntries: len(plan.Entries) > 0,
		PrevHref: weekHref(k.AddWeeks(-1)), NextHref: weekHref(k.AddWeeks(1)),
		StatusHref: fmt.Sprintf("/fragments/week-status?y=%d&w=%d", k.Year, k.Week),
		Errors:     e}
	if plan.Error != "" {
		v.Error = c.T(plan.Error)
	}
	for i := range 7 {
		day := monday.AddDate(0, 0, i)
		dc := plan.Context.Days[i]
		d := views.WeekDay{Index: i + 1, Name: c.WeekdayShort(day.Weekday()),
			Date: strconv.Itoa(day.Day()), Planned: !dc.Skip}
		if en, ok := plan.Entry(i + 1); ok {
			d.RecipeID, d.Title, d.Why = en.RecipeID, en.Title, en.Why
			d.Meta = c.T("recipes.minutes", "n", en.TotalMinutes) + ", " +
				c.N("recipes.servings", en.Servings)
		}
		d.SwapLabel = c.T("week.swap", "day", d.Name)
		v.Days = append(v.Days, d)
		cd := views.ContextDay{Index: i, Name: d.Name, Home: !dc.Skip, Busy: dc.Busy,
			Guests: strconv.Itoa(dc.Guests)}
		for _, m := range members {
			cd.Away = append(cd.Away, views.Option{Value: strconv.FormatInt(m.ID, 10),
				Label: m.Name, Checked: slices.Contains(dc.Away, m.ID)})
		}
		v.Context = append(v.Context, cd)
	}
	return v, true
}

func (s *server) weekStatus(w http.ResponseWriter, r *http.Request) {
	k, ok := s.weekKey(r)
	if !ok {
		s.badRequest(w, r)
		return
	}
	if !s.jobs.running(k) {
		w.Header().Set("HX-Refresh", "true")
		w.WriteHeader(http.StatusOK)
		return
	}
	href := fmt.Sprintf("/fragments/week-status?y=%d&w=%d", k.Year, k.Week)
	s.render(w, r, http.StatusOK, views.WeekStatus(href))
}

// editableWeek reads the form, the key and the plan (if any), refusing approved weeks
// with 409.
func (s *server) editableWeek(w http.ResponseWriter, r *http.Request) (weekplan.Key,
	weekplan.Plan, bool) {
	if !readForm(w, r) {
		return weekplan.Key{}, weekplan.Plan{}, false
	}
	k, ok := s.weekKey(r)
	if !ok {
		s.badRequest(w, r)
		return weekplan.Key{}, weekplan.Plan{}, false
	}
	p, err := s.Store.GetPlan(r.Context(), k)
	switch {
	case err == nil && p.Status == weekplan.StatusApproved:
		http.Error(w, i18n.T(r.Context(), "error.bad_request"), http.StatusConflict)
		return weekplan.Key{}, weekplan.Plan{}, false
	case err != nil && !errors.Is(err, store.ErrNotFound):
		s.fail(w, r, err)
		return weekplan.Key{}, weekplan.Plan{}, false
	}
	return k, p, true
}

func (s *server) saveWeekContext(w http.ResponseWriter, r *http.Request) {
	k, _, ok := s.editableWeek(w, r)
	if !ok {
		return
	}
	members, err := s.Store.ListMembers(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ids := make([]int64, len(members))
	for i, m := range members {
		ids[i] = m.ID
	}
	var c weekplan.Context
	e := validate.Errors{}
	for i := range 7 {
		f := func(name string) string { return validate.Field("days", i, name) }
		d := weekplan.DayContext{Skip: r.PostFormValue(f("home")) != "on",
			Busy: r.PostFormValue(f("busy")) == "on", Away: []int64{}}
		if g := strings.TrimSpace(r.PostFormValue(f("guests"))); g != "" {
			n, err := strconv.Atoi(g)
			if err != nil {
				e.Add(f("guests"), "form.not_a_number")
			}
			d.Guests = n
		}
		for _, a := range r.PostForm[f("away")] {
			id, err := strconv.ParseInt(a, 10, 64)
			if err != nil {
				e.Add(f("away"), "form.unknown_option")
				continue
			}
			d.Away = append(d.Away, id)
		}
		c.Days[i] = d
	}
	e.Merge(c.Validate(ids))
	if len(e) > 0 {
		v, ok := s.weekView(w, r, k, e)
		if !ok {
			return
		}
		s.render(w, r, http.StatusUnprocessableEntity, views.WeekPage(v))
		return
	}
	if err := s.Store.SaveContext(r.Context(), k, c); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, weekHref(k), http.StatusSeeOther)
}

func (s *server) generateWeek(w http.ResponseWriter, r *http.Request) {
	k, _, ok := s.editableWeek(w, r)
	if !ok {
		return
	}
	if !s.plannable(k) {
		s.badRequest(w, r)
		return
	}
	if s.Planner == nil {
		s.notFound(w, r)
		return
	}
	// The job must outlive this request, so it gets its own bounded context, not r.Context().
	//nolint:contextcheck // detached on purpose, see above
	s.jobs.start(k, func(ctx context.Context) error { return s.Planner.Generate(ctx, k) })
	http.Redirect(w, r, weekHref(k), http.StatusSeeOther)
}

func (s *server) swapDinner(w http.ResponseWriter, r *http.Request) {
	k, plan, ok := s.editableWeek(w, r)
	if !ok {
		return
	}
	day, err := strconv.Atoi(r.PostFormValue("day"))
	if err != nil || day < 1 || day > 7 || plan.Context.Days[day-1].Skip || !s.plannable(k) {
		s.badRequest(w, r)
		return
	}
	if s.Planner == nil {
		s.notFound(w, r)
		return
	}
	// The job must outlive this request, so it gets its own bounded context, not r.Context().
	//nolint:contextcheck // detached on purpose, see above
	s.jobs.start(k, func(ctx context.Context) error { return s.Planner.Swap(ctx, k, day) })
	http.Redirect(w, r, weekHref(k), http.StatusSeeOther)
}

func (s *server) approveWeek(w http.ResponseWriter, r *http.Request) {
	k, _, ok := s.editableWeek(w, r)
	if !ok {
		return
	}
	me, _ := auth.SessionFrom(r.Context())
	if err := s.Store.ApprovePlan(r.Context(), k, me.Subject); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, weekHref(k), http.StatusSeeOther)
}
