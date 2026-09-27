package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/recipes/importer"
	"github.com/aldersfors/matlistan/internal/store"
	"github.com/aldersfors/matlistan/internal/web/views"
)

// _importTimeout bounds an import, model calls included, below the server's 60 s write
// timeout, so the user always gets an answer.
var _importTimeout = 45 * time.Second

var _importErrors = []struct {
	err error
	key string
}{
	{context.DeadlineExceeded, "import.error.timeout"},
	{importer.ErrInvalidURL, "import.error.invalid_url"},
	{importer.ErrNotHTTPS, "import.error.not_https"},
	{importer.ErrNotAllowed, "import.error.not_allowed"},
	{importer.ErrTooLarge, "import.error.too_large"},
	{importer.ErrNotHTML, "import.error.not_html"},
	{importer.ErrNoRecipe, "import.error.no_recipe"},
	{importer.ErrModelUnavailable, "import.error.model_unavailable"},
	{importer.ErrUnreachable, "import.error.unreachable"},
}

func (s *server) importRecipe(w http.ResponseWriter, r *http.Request) {
	if !readForm(w, r) {
		return
	}
	if s.Importer == nil {
		s.notFound(w, r)
		return
	}
	raw := strings.TrimSpace(r.PostFormValue("url"))
	if strings.HasPrefix(strings.ToLower(raw), "http://") {
		raw = "https://" + raw[len("http://"):]
	}
	norm, err := recipes.NormalizeSourceURL(raw)
	if err != nil || len(raw) > recipes.SourceURLMax {
		s.importFailed(w, r, "import.error.invalid_url")
		return
	}
	if id, err := s.Store.FindRecipeBySourceURL(r.Context(), norm); err == nil {
		s.importFailed(w, r, "", fmt.Sprintf("/recipes/%d", id))
		return
	} else if !errors.Is(err, store.ErrNotFound) {
		s.fail(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), _importTimeout)
	defer cancel()
	rec, notes, err := s.Importer.Import(ctx, norm)
	if err != nil {
		key := "import.error.unreachable"
		for _, e := range _importErrors {
			if errors.Is(err, e.err) {
				key = e.key
				break
			}
		}
		// The error text holds the full URL, and a pasted link can carry a token: log the
		// host and the reason only.
		s.Log.Info().Str("host", hostOf(norm)).Str("reason", key).Msg("recipe import failed")
		s.importFailed(w, r, key)
		return
	}
	// The link may have redirected to a page imported before under another address.
	if rec.SourceURL != norm {
		if id, err := s.Store.FindRecipeBySourceURL(r.Context(), rec.SourceURL); err == nil {
			s.importFailed(w, r, "", fmt.Sprintf("/recipes/%d", id))
			return
		}
	}
	f := s.recipeForm(rec, nil)
	f.SourceURL = rec.SourceURL
	for _, n := range notes {
		f.Notes = append(f.Notes, s.Catalog.T(n))
	}
	s.render(w, r, http.StatusOK, views.RecipeFormPage(f))
}

// importFailed shows the library again with a message; existing is set for a duplicate.
func (s *server) importFailed(w http.ResponseWriter, r *http.Request, key string,
	existing ...string) {
	v, err := s.recipeListView(r, "")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if len(existing) > 0 {
		v.ImportDuplicate = existing[0]
	} else {
		v.ImportError = s.Catalog.T(key)
	}
	s.render(w, r, http.StatusUnprocessableEntity, views.RecipeListPage(v))
}

func hostOf(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		u = u[i+3:]
	}
	if i := strings.IndexByte(u, '/'); i >= 0 {
		u = u[:i]
	}
	return u
}
