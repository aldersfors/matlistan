package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jalet/matlistan/internal/household"
	"github.com/jalet/matlistan/internal/i18n"
)

func rateForm(day, member, score string) url.Values {
	return url.Values{"y": {"2026"}, "w": {"40"}, "day": {day}, "member": {member},
		"score": {score}}
}

func TestRatingScreen(t *testing.T) {
	h, st := approvedWeek(t)
	id, _ := st.CreateMember(t.Context(), household.Member{Name: "Leo", BirthYear: 2014})
	_, week := get(t, h, "/week?y=2026&w=40")
	if !strings.Contains(week, `href="/week/rate?y=2026&amp;w=40"`) {
		t.Fatal("approved week lacks the rating link")
	}
	_, body := get(t, h, "/week/rate?y=2026&w=40")
	for _, want := range []string{"Hur blev det?", "Pumpasoppa", "Köttbullar", "Leo", "Gott!",
		"Okej", "Inte igen"} {
		if !strings.Contains(body, want) {
			t.Errorf("rating screen lacks %q", want)
		}
	}
	rec := postHX(t, h, "/fragments/ratings", rateForm("1", itoa(id), "5"))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `aria-pressed="true"`) {
		t.Fatalf("rate: %d %s", rec.Code, rec.Body.String())
	}
	got, _ := st.WeekRatings(t.Context(), _w40)
	if got[1][id] != 5 {
		t.Fatalf("stored = %v", got)
	}
	if rec := post(t, h, "/fragments/ratings", rateForm("1", itoa(id), "3")); rec.Code != http.StatusSeeOther {
		t.Fatalf("rate without htmx: %d", rec.Code)
	}
	if got, _ := st.WeekRatings(t.Context(), _w40); got[1][id] != 3 {
		t.Fatalf("changed rating = %v", got)
	}
}

// Review focus 1: nothing to rate means 404; a bad score is 400.
func TestRatingRefusals(t *testing.T) {
	h, st := approvedWeek(t)
	id, _ := st.CreateMember(t.Context(), household.Member{Name: "Leo", BirthYear: 2014})
	for name, c := range map[string]struct {
		form url.Values
		want int
	}{
		"bad score":      {rateForm("1", itoa(id), "4"), http.StatusBadRequest},
		"no dinner":      {rateForm("3", itoa(id), "5"), http.StatusNotFound},
		"unknown member": {rateForm("1", "9999", "5"), http.StatusNotFound},
		"bad day":        {rateForm("9", itoa(id), "5"), http.StatusBadRequest},
	} {
		if rec := postHX(t, h, "/fragments/ratings", c.form); rec.Code != c.want {
			t.Errorf("%s: %d, want %d", name, rec.Code, c.want)
		}
	}
	draft := newServer(t, i18n.SV, true, newFakeStore())
	if res, _ := get(t, draft, "/week/rate?y=2026&w=40"); res.StatusCode != http.StatusNotFound {
		t.Errorf("rating page for an unapproved week: %d", res.StatusCode)
	}
}
