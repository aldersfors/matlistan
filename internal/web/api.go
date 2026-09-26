package web

import (
	"errors"
	"io"
	"net/http"

	"github.com/jalet/matlistan/internal/apitoken"
	"github.com/jalet/matlistan/internal/shopping"
	"github.com/jalet/matlistan/internal/store"
	"github.com/jalet/matlistan/internal/week"
	"github.com/jalet/matlistan/internal/weekplan"
)

// exportList serves the current list as text for the iOS Shortcut. It needs a valid key and
// says nothing about why a key was refused.
func (s *server) exportList(w http.ResponseWriter, r *http.Request) {
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
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, shopping.Text(s.Catalog, list))
}
