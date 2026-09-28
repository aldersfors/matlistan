package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/shopping"
	"github.com/aldersfors/matlistan/internal/store"
	"github.com/aldersfors/matlistan/internal/validate"
	"github.com/aldersfors/matlistan/internal/web/views"
	"github.com/aldersfors/matlistan/internal/week"
	"github.com/aldersfors/matlistan/internal/weekplan"
)

func (s *server) shoppingList(w http.ResponseWriter, r *http.Request) {
	var l shopping.List
	var err error
	if r.URL.Query().Get("y") != "" || r.URL.Query().Get("w") != "" {
		k, ok := s.weekKey(r)
		if !ok {
			s.badRequest(w, r)
			return
		}
		l, err = s.Store.GetShoppingList(r.Context(), k)
	} else {
		l, err = s.Store.CurrentShoppingList(r.Context(),
			weekplan.KeyOf(week.Upcoming(s.Now()).Days[0]))
	}
	switch {
	case errors.Is(err, store.ErrNotFound):
		s.render(w, r, http.StatusOK, views.ShoppingPage(views.Shopping{}))
		return
	case err != nil:
		s.fail(w, r, err)
		return
	}
	v := s.shoppingView(l, nil)
	v.Updated = r.URL.Query().Get("updated") == "1"
	s.render(w, r, http.StatusOK, views.ShoppingPage(v))
}

// rebuildList builds the approved week's list again, as at approval, from its dinners and
// the staples as they are now. The store keeps hand-added items and carries ticks.
func (s *server) rebuildList(w http.ResponseWriter, r *http.Request) {
	if !readForm(w, r) {
		return
	}
	k, ok := s.weekKey(r)
	if !ok {
		s.badRequest(w, r)
		return
	}
	uses, err := s.Store.PlanIngredients(r.Context(), k)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	staples, err := s.Store.ListStaples(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	items, excluded := shopping.Build(uses, staples)
	if err := s.Store.RebuildShoppingList(r.Context(), k, items, excluded); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/shopping?y=%d&w=%d&updated=1", k.Year, k.Week),
		http.StatusSeeOther)
}

func (s *server) shoppingView(l shopping.List, e validate.Errors) views.Shopping {
	c := s.Catalog
	monday := l.Key.Monday(s.Now().Location())
	v := views.Shopping{ListID: l.ID, Year: l.Key.Year, Week: l.Key.Week, HasList: true,
		Label:  c.WeekLabel(l.Key.Week),
		Range:  c.T("week.range", "from", c.Date(monday), "to", c.Date(monday.AddDate(0, 0, 6))),
		AtHome: c.N("shopping.at_home", l.Excluded), Errors: e}
	left := 0
	for _, it := range l.Items {
		if !it.Checked {
			left++
		}
		name := c.Section(it.Section)
		if n := len(v.Sections); n == 0 || v.Sections[n-1].Name != name {
			v.Sections = append(v.Sections, views.ShoppingSection{Name: name})
		}
		sec := &v.Sections[len(v.Sections)-1]
		sec.Items = append(sec.Items, s.shoppingRow(it))
	}
	v.Left = c.N("shopping.left", left)
	return v
}

func (s *server) shoppingRow(it shopping.Item) views.ShoppingRow {
	c := s.Catalog
	return views.ShoppingRow{ID: it.ID, Line: shopping.ItemLine(c, it), Days: it.Days,
		Checked: it.Checked, Manual: it.Manual,
		RemoveLabel: c.T("shopping.remove", "name", it.Name)}
}

func (s *server) toggleItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	if !readForm(w, r) {
		return
	}
	// The row posts the state it wants, so two people ticking at once agree.
	checked := r.PostFormValue("checked")
	if checked != "0" && checked != "1" {
		s.badRequest(w, r)
		return
	}
	it, err := s.Store.SetItemChecked(r.Context(), id, checked == "1")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if r.Header.Get("HX-Request") != "true" {
		http.Redirect(w, r, "/shopping", http.StatusSeeOther)
		return
	}
	s.render(w, r, http.StatusOK, views.ShoppingRowView(s.shoppingRow(it)))
}

func (s *server) addItem(w http.ResponseWriter, r *http.Request) {
	if !readForm(w, r) {
		return
	}
	listID, err := strconv.ParseInt(r.PostFormValue("list"), 10, 64)
	if err != nil {
		s.badRequest(w, r)
		return
	}
	name := household.StapleName(r.PostFormValue("name"))
	e := validate.Errors{}
	e.Text("name", name, 1, 80)
	if len(e) == 0 {
		err = s.Store.AddManualItem(r.Context(), listID, name)
		if errors.Is(err, store.ErrTooMany) {
			e.Add("name", "shopping.too_many")
		} else if err != nil {
			s.fail(w, r, err)
			return
		}
	}
	if len(e) > 0 {
		l, err := s.Store.CurrentShoppingList(r.Context(),
			weekplan.KeyOf(week.Upcoming(s.Now()).Days[0]))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		s.render(w, r, http.StatusUnprocessableEntity, views.ShoppingPage(s.shoppingView(l, e)))
		return
	}
	http.Redirect(w, r, "/shopping", http.StatusSeeOther)
}

func (s *server) removeItem(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	if err := s.Store.RemoveManualItem(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/shopping", http.StatusSeeOther)
}
