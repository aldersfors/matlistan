package web

import "net/http"

// locked answers a change to a row that git owns: the change belongs in git.
func (s *server) locked(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, s.Catalog.T("household.locked"), http.StatusConflict)
}
