package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
)

func settingsForm(dinners string) url.Values {
	return url.Values{"dinners_per_week": {dinners}, "weeknight_minutes": {"30"},
		"repeat_window_weeks": {"8"}, "library_share": {"60"}}
}

func TestSaveSettings(t *testing.T) {
	st := newFakeStore()
	h := newServer(t, i18n.SV, true, st)
	rec := post(t, h, "/settings", settingsForm("5"))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/settings?saved=1" {
		t.Fatalf("status %d location %q", rec.Code, rec.Header().Get("Location"))
	}
	want := household.Settings{DinnersPerWeek: 5, WeeknightMinutes: 30, RepeatWindowWeeks: 8,
		LibraryShare: 60}
	if got, _ := st.GetSettings(t.Context()); got != want {
		t.Fatalf("settings = %+v", got)
	}
	_, body := get(t, h, "/settings?saved=1")
	if !strings.Contains(body, "Inställningarna sparades.") || !strings.Contains(body, `value="5"`) {
		t.Fatal("confirmation or saved value missing")
	}
}

func TestSettingsRejectsBadValues(t *testing.T) {
	st := newFakeStore()
	h := newServer(t, i18n.SV, true, st)
	rec := post(t, h, "/settings", settingsForm("9"))
	if rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "Utanför tillåtet intervall") ||
		!strings.Contains(rec.Body.String(), `value="9"`) {
		t.Fatalf("status %d", rec.Code)
	}
	if got, _ := st.GetSettings(t.Context()); got != household.DefaultSettings() {
		t.Fatal("invalid settings were stored")
	}
	if rec := post(t, h, "/settings", settingsForm("fem")); !strings.Contains(rec.Body.String(),
		"Inte ett tal") {
		t.Fatal("non-number accepted")
	}
}

func TestStaples(t *testing.T) {
	st := newFakeStore()
	h := newServer(t, i18n.SV, true, st)
	for _, n := range []string{"Salt", " salt ", "olja"} {
		if rec := post(t, h, "/settings/staples", url.Values{"name": {n}}); rec.Code != http.StatusSeeOther {
			t.Fatalf("add %q: %d", n, rec.Code)
		}
	}
	if rec := post(t, h, "/settings/staples", url.Values{"name": {"   "}}); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blank staple: %d", rec.Code)
	}
	_, body := get(t, h, "/settings")
	if strings.Count(body, "Ta bort Salt") != 1 || !strings.Contains(body, "olja") {
		t.Fatalf("staples not listed once each")
	}
	list, _ := st.ListStaples(t.Context())
	if rec := post(t, h, "/settings/staples/"+itoa(list[0].ID)+"/delete", nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := post(t, h, "/settings/staples/999/delete", nil); rec.Code != http.StatusNotFound {
		t.Fatalf("delete unknown: %d", rec.Code)
	}
}
