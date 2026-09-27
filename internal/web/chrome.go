package web

import (
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/aldersfors/matlistan/internal/auth"
	"github.com/aldersfors/matlistan/internal/release"
	"github.com/aldersfors/matlistan/internal/web/views"
)

// _sourceRepo is where released commits live; the footer links the running commit there.
const _sourceRepo = "https://github.com/aldersfors/matlistan"

var _sha = regexp.MustCompile(`^[0-9a-f]{40}$`)

// viewer is the signed-in person: the family member linked to this login when there is
// one, otherwise the IdP's name, then the email. nil without a session.
func (s *server) viewer(r *http.Request) *views.Viewer {
	me, ok := auth.SessionFrom(r.Context())
	if !ok || me.Subject == "" {
		return nil
	}
	name := strings.TrimSpace(me.Name)
	if members, err := s.Store.ListMembers(r.Context()); err == nil {
		for _, m := range members {
			if m.Subject == me.Subject {
				name = m.Name
				break
			}
		}
	}
	name = firstNonEmpty(name, me.Email, "?")
	first, _ := utf8.DecodeRuneInString(name)
	v := &views.Viewer{Initial: string(unicode.ToUpper(first)), Name: name, Email: me.Email}
	if v.Email == v.Name {
		v.Email = "" // the email already is the name
	}
	return v
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// footerBuild formats b for the footer, with times in the app's time zone.
func (s *server) footerBuild(b release.Info) views.Build {
	f := views.Build{Version: b.Version, Short: b.ShortCommit(), Dirty: b.Dirty}
	if f.Version != "dev" {
		f.Version = "v" + f.Version
	}
	if _sha.MatchString(b.Commit) && !b.Dirty {
		f.URL = _sourceRepo + "/commit/" + b.Commit
	}
	f.Committed, f.CommittedISO = s.footerTime(b.CommitTime)
	f.Built, f.BuiltISO = s.footerTime(b.BuildTime)
	return f
}

func (s *server) footerTime(t time.Time) (string, string) {
	if t.IsZero() {
		return "", ""
	}
	local := t.In(s.Now().Location())
	return s.Catalog.Date(local) + " " + local.Format("15:04"), t.UTC().Format(time.RFC3339)
}
