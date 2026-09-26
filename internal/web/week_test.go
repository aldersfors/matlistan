package web

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/jalet/matlistan/internal/household"
	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/weekplan"
)

var _w40 = weekplan.Key{Year: 2026, Week: 40}

func weekForm(extra url.Values) url.Values {
	v := url.Values{"y": {"2026"}, "w": {"40"}}
	for k, vs := range extra {
		v[k] = vs
	}
	return v
}

func TestWeekWithoutPlannerSaysSo(t *testing.T) {
	_, body := get(t, newServer(t, i18n.SV, true, newFakeStore()), "/week")
	if !strings.Contains(body, "Planeringen är inte inställd") ||
		strings.Contains(body, "Planera veckan</button>") {
		t.Fatal("unconfigured planner not shown")
	}
}

func TestGenerateShowsProgressThenThePlan(t *testing.T) {
	st := newFakeStore()
	pl := &fakePlanner{st: st, gate: make(chan struct{})}
	h, s := newPlanningServer(t, i18n.SV, st, pl)
	rec := post(t, h, "/week/generate", weekForm(nil))
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/week?y=2026&w=40" {
		t.Fatalf("generate: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// Review focus 3: a second press while running starts nothing new.
	post(t, h, "/week/generate", weekForm(nil))
	_, body := get(t, h, "/week?y=2026&w=40")
	if !strings.Contains(body, "Planerar veckan") || !strings.Contains(body,
		`hx-get="/fragments/week-status?y=2026&amp;w=40"`) {
		t.Fatalf("progress not shown: %.300s", body)
	}
	res, _ := get(t, h, "/fragments/week-status?y=2026&w=40")
	if res.Header.Get("HX-Refresh") != "" {
		t.Fatal("refresh while still running")
	}
	close(pl.gate)
	s.jobs.wait()
	res, _ = get(t, h, "/fragments/week-status?y=2026&w=40")
	if res.Header.Get("HX-Refresh") != "true" {
		t.Fatal("no refresh after the job ended")
	}
	_, body = get(t, h, "/week?y=2026&w=40")
	for _, want := range []string{"Pumpasoppa", "Pumpan är i säsong.", "Förslag",
		"Godkänn veckan", "Byt middag på Mån", "30 min, 4 portioner"} {
		if !strings.Contains(body, want) {
			t.Errorf("draft lacks %q", want)
		}
	}
}

// Review focus 1: a failed run shows the localized error and offers to try again.
func TestFailedGenerationIsShown(t *testing.T) {
	st := newFakeStore()
	pl := &fakePlanner{st: st, err: errors.New("invalid")}
	h, s := newPlanningServer(t, i18n.SV, st, pl)
	post(t, h, "/week/generate", weekForm(nil))
	s.jobs.wait()
	_, body := get(t, h, "/week?y=2026&w=40")
	if !strings.Contains(body, "Planeraren hittade ingen vecka") ||
		!strings.Contains(body, "Planera veckan") {
		t.Fatal("error or retry missing")
	}
}

func TestSaveConditions(t *testing.T) {
	st := newFakeStore()
	id, _ := st.CreateMember(t.Context(), household.Member{Name: "Erik", BirthYear: 1984})
	h, _ := newPlanningServer(t, i18n.SV, st, &fakePlanner{st: st})
	rec := post(t, h, "/week/context", weekForm(url.Values{
		"days.0.home": {"on"}, "days.1.home": {"on"}, "days.1.busy": {"on"},
		"days.2.home": {"on"}, "days.2.guests": {"2"}, "days.2.away": {itoa(id)}}))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d body %.200s", rec.Code, rec.Body.String())
	}
	p, _ := st.GetPlan(t.Context(), _w40)
	d := p.Context.Days
	if d[0].Skip || !d[1].Busy || d[2].Guests != 2 || len(d[2].Away) != 1 || !d[3].Skip {
		t.Fatalf("context = %+v", d)
	}
	if rec := post(t, h, "/week/context", weekForm(url.Values{"days.0.guests": {"99"}})); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad guests: %d", rec.Code)
	}
}

// Review focus 4: swap and approve; approved weeks refuse changes.
func TestSwapAndApprove(t *testing.T) {
	st := newFakeStore()
	pl := &fakePlanner{st: st}
	h, s := newPlanningServer(t, i18n.SV, st, pl)
	post(t, h, "/week/generate", weekForm(nil))
	s.jobs.wait()
	if rec := post(t, h, "/week/swap", weekForm(url.Values{"day": {"2"}})); rec.Code != http.StatusSeeOther {
		t.Fatalf("swap: %d", rec.Code)
	}
	s.jobs.wait()
	if len(pl.swapped) != 1 || pl.swapped[0] != 2 {
		t.Fatalf("swapped %v", pl.swapped)
	}
	if rec := post(t, h, "/week/swap", weekForm(url.Values{"day": {"9"}})); rec.Code != http.StatusBadRequest {
		t.Fatalf("swap day 9: %d", rec.Code)
	}
	c := weekplan.DefaultContext(7)
	c.Days[2].Skip = true
	if err := st.SaveContext(t.Context(), _w40, c); err != nil {
		t.Fatal(err)
	}
	if rec := post(t, h, "/week/swap", weekForm(url.Values{"day": {"3"}})); rec.Code != http.StatusBadRequest {
		t.Fatalf("swap a day without dinner: %d", rec.Code)
	}
	if rec := post(t, h, "/week/approve", weekForm(nil)); rec.Code != http.StatusSeeOther {
		t.Fatalf("approve: %d", rec.Code)
	}
	p, _ := st.GetPlan(t.Context(), _w40)
	if p.Status != weekplan.StatusApproved || p.ApprovedBy != "sub-anna" {
		t.Fatalf("plan = %+v", p)
	}
	for _, path := range []string{"/week/swap", "/week/generate", "/week/context"} {
		if rec := post(t, h, path, weekForm(url.Values{"day": {"1"}})); rec.Code != http.StatusConflict {
			t.Errorf("%s on approved week: %d", path, rec.Code)
		}
	}
	_, body := get(t, h, "/week?y=2026&w=40")
	if !strings.Contains(body, "Godkänd") || strings.Contains(body, "Byt middag") {
		t.Fatal("approved week still editable")
	}
}

// Review focus 5: navigation across the turn of the year; bad keys are 400.
func TestWeekNavigation(t *testing.T) {
	h := newServer(t, i18n.SV, true, newFakeStore())
	_, body := get(t, h, "/week?y=2026&w=53")
	if !strings.Contains(body, `href="/week?y=2026&amp;w=52"`) ||
		!strings.Contains(body, `href="/week?y=2027&amp;w=1"`) || !strings.Contains(body, "Vecka 53") {
		t.Fatalf("navigation: %.400s", body)
	}
	for _, q := range []string{"y=2027&w=53", "y=abc&w=1", "y=2026"} {
		if res, _ := get(t, h, "/week?"+q); res.StatusCode != http.StatusBadRequest {
			t.Errorf("%s: %d", q, res.StatusCode)
		}
	}
}

// Planning costs money, so only the current week and the next few can be planned.
func TestPlanningOnlyForNearWeeks(t *testing.T) {
	st := newFakeStore()
	h, s := newPlanningServer(t, i18n.SV, st, &fakePlanner{st: st})
	for _, yw := range [][2]string{{"2026", "38"}, {"2027", "10"}, {"2099", "1"}} {
		rec := post(t, h, "/week/generate", url.Values{"y": {yw[0]}, "w": {yw[1]}})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("generate %s-W%s: %d, want 400", yw[0], yw[1], rec.Code)
		}
	}
	for _, w := range []string{"39", "40", "47"} { // current week, next, current + 8
		if rec := post(t, h, "/week/generate", url.Values{"y": {"2026"}, "w": {w}}); rec.Code != http.StatusSeeOther {
			t.Errorf("generate 2026-W%s: %d, want 303", w, rec.Code)
		}
		s.jobs.wait()
	}
	if _, body := get(t, h, "/week?y=2027&w=10"); strings.Contains(body, "Planera veckan") {
		t.Error("a week outside the planning window offers planning")
	}
}

func TestFarWeekDoesNotClaimPlanningIsOff(t *testing.T) {
	st := newFakeStore()
	h, _ := newPlanningServer(t, i18n.SV, st, &fakePlanner{st: st})
	if _, body := get(t, h, "/week?y=2027&w=10"); strings.Contains(body, "inte inställd") {
		t.Fatal("configured planner reported as not set up")
	}
}
