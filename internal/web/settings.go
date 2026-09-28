package web

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/aldersfors/matlistan/internal/apitoken"
	"github.com/aldersfors/matlistan/internal/auth"
	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/validate"
	"github.com/aldersfors/matlistan/internal/web/views"
)

var _settingFields = []string{"dinners_per_week", "weeknight_minutes", "repeat_window_weeks",
	"library_share"}

func (s *server) settings(w http.ResponseWriter, r *http.Request) {
	st, err := s.Store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v, ok := s.settingsView(w, r, st, nil)
	if !ok {
		return
	}
	v.Saved = r.URL.Query().Get("saved") == "1"
	s.render(w, r, http.StatusOK, views.SettingsPage(v))
}

// settingsView fills the page, loading the staples; false means the error is written.
func (s *server) settingsView(w http.ResponseWriter, r *http.Request, st household.Settings,
	e validate.Errors) (views.Settings, bool) {
	staples, err := s.Store.ListStaples(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return views.Settings{}, false
	}
	v := views.Settings{DinnersPerWeek: strconv.Itoa(st.DinnersPerWeek),
		WeeknightMinutes:  strconv.Itoa(st.WeeknightMinutes),
		RepeatWindowWeeks: strconv.Itoa(st.RepeatWindowWeeks),
		LibraryShare:      strconv.Itoa(st.LibraryShare), Errors: e}
	for _, sp := range staples {
		v.Staples = append(v.Staples, views.StapleRow{ID: sp.ID, Name: sp.Name})
	}
	tokens, err := s.Store.ListAPITokens(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return views.Settings{}, false
	}
	c := s.Catalog
	for _, t := range tokens {
		used := c.T("tokens.never_used")
		if t.LastUsedAt != nil {
			used = c.T("tokens.last_used", "date", c.Date(t.LastUsedAt.In(s.Now().Location())))
		}
		v.Tokens = append(v.Tokens, views.TokenRow{ID: t.ID, Name: t.Name, Used: used,
			RevokeLabel: c.T("tokens.revoke", "name", t.Name)})
	}
	v.ExportURL = strings.TrimSuffix(s.BaseURL, "/") + "/api/v1/shopping-list/current.md"
	if s.PushKey != "" {
		n, err := s.Store.CountPushSubscriptions(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return views.Settings{}, false
		}
		v.PushKey, v.PushDevices = s.PushKey, c.N("push.devices", n)
	}
	return v, true
}

func (s *server) saveSettings(w http.ResponseWriter, r *http.Request) {
	if !readForm(w, r) {
		return
	}
	e := validate.Errors{}
	vals := make([]int, len(_settingFields))
	for i, f := range _settingFields {
		n, err := strconv.Atoi(strings.TrimSpace(r.PostFormValue(f)))
		if err != nil {
			e.Add(f, "form.not_a_number")
		}
		vals[i] = n
	}
	st := household.Settings{DinnersPerWeek: vals[0], WeeknightMinutes: vals[1],
		RepeatWindowWeeks: vals[2], LibraryShare: vals[3]}
	e.Merge(st.Validate())
	if len(e) > 0 {
		v, ok := s.settingsView(w, r, st, e)
		if !ok {
			return
		}
		// Show what was typed, not the parsed zeros.
		v.DinnersPerWeek = r.PostFormValue("dinners_per_week")
		v.WeeknightMinutes = r.PostFormValue("weeknight_minutes")
		v.RepeatWindowWeeks = r.PostFormValue("repeat_window_weeks")
		v.LibraryShare = r.PostFormValue("library_share")
		s.render(w, r, http.StatusUnprocessableEntity, views.SettingsPage(v))
		return
	}
	if err := s.Store.UpdateSettings(r.Context(), st); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings?saved=1", http.StatusSeeOther)
}

func (s *server) addStaple(w http.ResponseWriter, r *http.Request) {
	if !readForm(w, r) {
		return
	}
	name := household.StapleName(r.PostFormValue("name"))
	e := validate.Errors{}
	e.Text("staple", name, 1, 80)
	if len(e) > 0 {
		st, err := s.Store.GetSettings(r.Context())
		if err != nil {
			s.fail(w, r, err)
			return
		}
		v, ok := s.settingsView(w, r, st, e)
		if !ok {
			return
		}
		s.render(w, r, http.StatusUnprocessableEntity, views.SettingsPage(v))
		return
	}
	if err := s.Store.AddStaple(r.Context(), name); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *server) removeStaple(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	if err := s.Store.RemoveStaple(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}

func (s *server) createToken(w http.ResponseWriter, r *http.Request) {
	if !readForm(w, r) {
		return
	}
	name := household.StapleName(r.PostFormValue("name"))
	e := validate.Errors{}
	e.Text("token_name", name, 1, 60)
	var plain string
	if len(e) == 0 {
		var hash []byte
		var err error
		if plain, hash, err = apitoken.New(); err != nil {
			s.fail(w, r, err)
			return
		}
		me, _ := auth.SessionFrom(r.Context())
		if err := s.Store.CreateAPIToken(r.Context(), me.Subject, name, hash); err != nil {
			s.fail(w, r, err)
			return
		}
	}
	st, err := s.Store.GetSettings(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	v, ok := s.settingsView(w, r, st, e)
	if !ok {
		return
	}
	v.NewToken = plain
	w.Header().Set("Cache-Control", "no-store")
	status := http.StatusOK
	if len(e) > 0 {
		status = http.StatusUnprocessableEntity
	}
	s.render(w, r, status, views.SettingsPage(v))
}

func (s *server) revokeToken(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		s.notFound(w, r)
		return
	}
	if err := s.Store.RevokeAPIToken(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/settings", http.StatusSeeOther)
}
