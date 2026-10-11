package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/aldersfors/matlistan/internal/store"
	"github.com/aldersfors/matlistan/internal/validate"
	"github.com/aldersfors/matlistan/internal/web/views"
	"github.com/aldersfors/matlistan/internal/weekplan"
)

// dayToSet reads the week and day whose dinner is set by hand, with the week's plan (a week
// without a row gets the household's default days). The week must be a draft in the planning
// window that is not being planned, and the day one with dinner at home. False means the
// response is written.
func (s *server) dayToSet(w http.ResponseWriter, r *http.Request) (weekplan.Key,
	weekplan.Plan, int, bool) {
	k, plan, ok := s.editableWeek(w, r)
	if !ok {
		return weekplan.Key{}, weekplan.Plan{}, 0, false
	}
	if plan.Status == "" { // no row yet
		settings, err := s.Store.GetSettings(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return weekplan.Key{}, weekplan.Plan{}, 0, false
		}
		plan = weekplan.Plan{Key: k, Status: weekplan.StatusDraft,
			Context: weekplan.DefaultContext(settings.DinnersPerWeek)}
	}
	day, err := strconv.Atoi(r.FormValue("day"))
	if err != nil || day < 1 || day > 7 || !s.plannable(k) || s.jobs.running(k) ||
		plan.Context.Days[day-1].Skip {
		s.badRequest(w, r)
		return weekplan.Key{}, weekplan.Plan{}, 0, false
	}
	return k, plan, day, true
}

func (s *server) weekDay(w http.ResponseWriter, r *http.Request) {
	k, plan, day, ok := s.dayToSet(w, r)
	if !ok {
		return
	}
	v, err := s.weekDayView(r, k, plan, day, r.FormValue("q"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	s.render(w, r, http.StatusOK, views.WeekDayPage(v))
}

// weekDayView is the day page with the library matching raw.
func (s *server) weekDayView(r *http.Request, k weekplan.Key, plan weekplan.Plan, day int,
	raw string) (views.WeekDayEdit, error) {
	list, err := s.recipeListView(r, raw)
	if err != nil {
		return views.WeekDayEdit{}, err
	}
	c := s.Catalog
	date := k.Monday(s.Now().Location()).AddDate(0, 0, day-1)
	name := c.WeekdayShort(date.Weekday()) + " " + strconv.Itoa(date.Day())
	v := views.WeekDayEdit{Year: k.Year, Number: k.Week, Day: day,
		Heading:   c.T("week.day.title", "day", name),
		BackLabel: c.WeekLabel(k.Week), BackHref: weekHref(k),
		Query: list.Query, Recipes: list.Items}
	if e, ok := plan.Entry(day); ok {
		v.HasDinner = true
		if e.Custom() {
			v.Title = e.Title
		}
	}
	return v, nil
}

// setDay sets, changes or removes a day's dinner by hand; a dinner set by hand is locked.
func (s *server) setDay(w http.ResponseWriter, r *http.Request) {
	k, plan, day, ok := s.dayToSet(w, r)
	if !ok {
		return
	}
	var err error
	switch {
	case r.PostFormValue("clear") == "1":
		err = s.Store.ClearEntry(r.Context(), k, day)
	case r.PostFormValue("recipe") != "":
		id, perr := strconv.ParseInt(r.PostFormValue("recipe"), 10, 64)
		if perr != nil || id <= 0 {
			s.badRequest(w, r)
			return
		}
		if _, gerr := s.Store.GetRecipe(r.Context(), id); errors.Is(gerr, store.ErrNotFound) {
			s.badRequest(w, r)
			return
		} else if gerr != nil {
			s.fail(w, r, gerr)
			return
		}
		err = s.setEntry(r, k, plan, day, id, "")
	default:
		title := weekplan.TidyTitle(r.PostFormValue("title"))
		e := validate.Errors{}
		e.Text("title", title, 1, weekplan.TitleMaxRunes)
		if len(e) > 0 {
			v, verr := s.weekDayView(r, k, plan, day, "")
			if verr != nil {
				s.fail(w, r, verr)
				return
			}
			v.Title, v.Errors = r.PostFormValue("title"), e // what was typed, so nothing is lost
			s.render(w, r, http.StatusUnprocessableEntity, views.WeekDayPage(v))
			return
		}
		err = s.setEntry(r, k, plan, day, 0, title)
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.badRequest(w, r)
		return
	case errors.Is(err, store.ErrApproved): // approved since the form was read
		http.Error(w, s.Catalog.T("error.bad_request"), http.StatusConflict)
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	s.live.publish(k)
	http.Redirect(w, r, weekHref(k), http.StatusSeeOther)
}

// setEntry stores the dinner with the servings the day's conditions give, as planning would.
func (s *server) setEntry(r *http.Request, k weekplan.Key, plan weekplan.Plan, day int,
	recipeID int64, title string) error {
	members, err := s.Store.ListMembers(r.Context())
	if err != nil {
		return err
	}
	settings, err := s.Store.GetSettings(r.Context())
	if err != nil {
		return err
	}
	spec := weekplan.Specs(k, s.Now().Location(), plan.Context, members, settings)[day-1]
	return s.Store.SetEntry(r.Context(), k, plan.Context, day, recipeID, title, spec.Servings)
}
