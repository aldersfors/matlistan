package web

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/jalet/matlistan/internal/i18n"
)

func (s *server) render(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		s.Log.Error().Err(err).Str("path", r.URL.Path).Msg("render")
	}
}

func (s *server) notFound(w http.ResponseWriter, r *http.Request) {
	http.Error(w, i18n.T(r.Context(), "error.not_found"), http.StatusNotFound)
}
