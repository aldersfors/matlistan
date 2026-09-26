package web

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/a-h/templ"

	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/store"
)

// formBytesMax bounds a form post; the largest form (a full recipe) is well under it.
const formBytesMax = 64 << 10

// readForm parses a bounded POST body. false means the error response is already written.
func readForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, formBytesMax)
	if err := r.ParseForm(); err != nil || !storableForm(r.PostForm) {
		http.Error(w, i18n.T(r.Context(), "error.bad_request"), http.StatusBadRequest)
		return false
	}
	return true
}

// storable reports whether Postgres can keep s as text: valid UTF-8 without NUL bytes.
func storable(s string) bool { return utf8.ValidString(s) && !strings.ContainsRune(s, 0) }

func storableForm(v url.Values) bool {
	for _, vs := range v {
		for _, s := range vs {
			if !storable(s) {
				return false
			}
		}
	}
	return true
}

// pathID reads the {id} path value; false for anything but a positive integer.
func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (s *server) render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		s.Log.Error().Err(err).Str("path", r.URL.Path).Msg("render")
	}
}

// fail answers 404 for missing rows and 500 for everything else, logging only the latter.
func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, store.ErrNotFound) {
		http.Error(w, i18n.T(r.Context(), "error.not_found"), http.StatusNotFound)
		return
	}
	s.Log.Error().Err(err).Str("path", r.URL.Path).Msg("request failed")
	http.Error(w, i18n.T(r.Context(), "error.internal"), http.StatusInternalServerError)
}

func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	http.Error(w, i18n.T(r.Context(), "error.not_found"), http.StatusNotFound)
}
