package declared

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog"

	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/recipes/importer"
	"github.com/aldersfors/matlistan/internal/store"
)

// LinkImporter reads a recipe page; importer.Importer is one.
type LinkImporter interface {
	Import(ctx context.Context, url string) (recipes.Recipe, []string, error)
}

// LinkStore saves recipes imported from declared links.
type LinkStore interface {
	CreateLinkRecipe(ctx context.Context, r recipes.Recipe) (int64, error)
	AdoptLink(ctx context.Context, link string) error
}

// Links imports declared links that have no recipe yet, one at a time. Transient failures
// are retried every Retry; permanent ones wait for the next start.
type Links struct {
	Importer LinkImporter
	Store    LinkStore
	Log      zerolog.Logger
	Retry    time.Duration // time between rounds; an hour in production
	Timeout  time.Duration // for one link; two minutes in production
}

// _attemptsMax bounds the tries per link and start: a page the model always fails on must
// not cost a model call every hour for as long as the pod runs.
const _attemptsMax = 3

// Run returns when every link is imported or dropped, or when ctx ends.
func (l Links) Run(ctx context.Context, links []string) {
	pending := append([]string(nil), links...)
	for round := 1; len(pending) > 0; round++ {
		var again []string
		for _, link := range pending {
			if ctx.Err() != nil {
				return
			}
			if l.one(ctx, link, round < _attemptsMax) {
				again = append(again, link)
			}
		}
		pending = again
		if len(pending) == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(l.Retry):
		}
	}
}

// one imports link and reports whether to try again later; mayRetry is false on the last
// attempt.
func (l Links) one(ctx context.Context, link string, mayRetry bool) bool {
	host := hostOf(link)
	ctx, cancel := context.WithTimeout(ctx, l.Timeout)
	defer cancel()
	r, _, err := l.Importer.Import(ctx, link)
	if err != nil {
		retry := mayRetry && transient(err)
		l.Log.Warn().Str("host", host).Str("reason", reason(err)).Bool("retry", retry).
			Msg("recipe link import failed")
		return retry
	}
	r.SourceURL = link // the declared link is the identity, whatever the page redirected to
	if e := r.Validate(); len(e) > 0 {
		fields := make([]string, 0, len(e))
		for f := range e {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		l.Log.Warn().Str("host", host).Str("reason", "invalid").Str("fields",
			strings.Join(fields, ",")).Bool("retry", false).Msg("recipe link import failed")
		return false
	}
	_, err = l.Store.CreateLinkRecipe(ctx, r)
	if errors.Is(err, store.ErrDuplicateSource) { // imported in the app meanwhile
		err = l.Store.AdoptLink(ctx, link)
	}
	if err != nil {
		l.Log.Warn().Str("host", host).Str("reason", "store").Bool("retry", mayRetry).
			Msg("recipe link import failed")
		return mayRetry
	}
	l.Log.Info().Str("host", host).Msg("recipe link imported")
	return false
}

func transient(err error) bool {
	return errors.Is(err, importer.ErrUnreachable) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, importer.ErrModelUnavailable)
}

func reason(err error) string {
	for _, c := range []struct {
		err error
		key string
	}{{importer.ErrUnreachable, "unreachable"}, {importer.ErrNoRecipe, "no_recipe"},
		{importer.ErrNotAllowed, "not_allowed"}, {importer.ErrNotHTML, "not_html"},
		{importer.ErrTooLarge, "too_large"}, {importer.ErrInvalidURL, "invalid_url"},
		{importer.ErrNotHTTPS, "not_https"}, {context.DeadlineExceeded, "timeout"},
		{importer.ErrModelUnavailable, "model_unavailable"}} {
		if errors.Is(err, c.err) {
			return c.key
		}
	}
	return "other"
}

func hostOf(link string) string {
	if u, err := url.Parse(link); err == nil && u.Host != "" {
		return u.Hostname()
	}
	return "unknown host"
}
