package web

import (
	"errors"
	"io"
	"net/http"

	"github.com/aldersfors/matlistan/internal/apitoken"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/shopping"
	"github.com/aldersfors/matlistan/internal/store"
	"github.com/aldersfors/matlistan/internal/week"
	"github.com/aldersfors/matlistan/internal/weekplan"
)

// exportList serves the current list as text for the iOS Shortcut, exportHTML as rich text
// for Apple Notes and exportJSON as sections of lines for the checklist Shortcut. All need a
// valid key and are never cached.
func (s *server) exportList(w http.ResponseWriter, r *http.Request) {
	s.export(w, r, "text/plain; charset=utf-8", shopping.Text)
}

func (s *server) exportHTML(w http.ResponseWriter, r *http.Request) {
	s.export(w, r, "text/html; charset=utf-8", shopping.HTML)
}

func (s *server) exportJSON(w http.ResponseWriter, r *http.Request) {
	s.export(w, r, "application/json", shopping.JSON)
}

func (s *server) export(w http.ResponseWriter, r *http.Request, contentType string,
	render func(*i18n.Catalog, *shopping.List) string) {
	tok, ok := apitoken.FromHeader(r.Header.Get("Authorization"))
	if ok {
		found, err := s.Store.UseAPIToken(r.Context(), apitoken.Hash(tok))
		if err != nil {
			s.fail(w, r, err)
			return
		}
		ok = found
	}
	if !ok {
		w.Header().Set("WWW-Authenticate", `Bearer realm="matlistan"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	l, err := s.Store.CurrentShoppingList(r.Context(),
		weekplan.KeyOf(week.Upcoming(s.Now()).Days[0]))
	var list *shopping.List
	switch {
	case err == nil:
		list = &l
	case !errors.Is(err, store.ErrNotFound):
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, render(s.Catalog, list))
}
