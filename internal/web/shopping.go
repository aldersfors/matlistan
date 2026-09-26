package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/jalet/matlistan/internal/household"
	"github.com/jalet/matlistan/internal/shopping"
	"github.com/jalet/matlistan/internal/store"
	"github.com/jalet/matlistan/internal/validate"
	"github.com/jalet/matlistan/internal/web/views"
	"github.com/jalet/matlistan/internal/week"
	"github.com/jalet/matlistan/internal/weekplan"
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
	s.render(w, r, http.StatusOK, views.ShoppingPage(s.shoppingView(l, nil)))
}

func (s *server) shoppingView(l shopping.List, e validate.Errors) views.Shopping {
	c := s.Catalog
	monday := l.Key.Monday(s.Now().Location())
	v := views.Shopping{ListID: l.ID, HasList: true, Label: c.WeekLabel(l.Key.Week),
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
