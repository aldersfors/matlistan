package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/weekplan"
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
		"Godkänn veckan", "Lås middagen på Mån", "30 min, 4 portioner"} {
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

// Review focus 4: lock and approve; approved weeks refuse changes.
func TestLockAndApprove(t *testing.T) {
	st := newFakeStore()
	pl := &fakePlanner{st: st}
	h, s := newPlanningServer(t, i18n.SV, st, pl)
	post(t, h, "/week/generate", weekForm(nil))
	s.jobs.wait()
	if _, body := get(t, h, "/week?y=2026&w=40"); !strings.Contains(body, "Planera om hela veckan") ||
		strings.Count(body, `aria-pressed="false"`) != 2 {
		t.Fatalf("unlocked week:\n%s", body)
	}
	lock := func(day, locked string) int {
		return post(t, h, "/week/lock", weekForm(url.Values{"day": {day}, "locked": {locked}})).Code
	}
	if code := lock("2", "1"); code != http.StatusSeeOther {
		t.Fatalf("lock: %d", code)
	}
	if e, _ := st.plans[_w40].Entry(2); !e.Locked {
		t.Fatal("day 2 not locked")
	}
	_, body := get(t, h, "/week?y=2026&w=40")
	if !strings.Contains(body, "Planera om olåsta") || !strings.Contains(body, "Lås middagen på Tis") ||
		strings.Count(body, `aria-pressed="true"`) != 1 {
		t.Fatalf("one locked:\n%s", body)
	}
	for _, day := range []string{"9", "3", "x"} { // day 3 has no dinner in the fake
		if code := lock(day, "1"); code != http.StatusBadRequest {
			t.Errorf("lock day %s: %d", day, code)
		}
	}
	if code := lock("2", "0"); code != http.StatusSeeOther {
		t.Fatalf("unlock: %d", code)
	}
	if e, _ := st.plans[_w40].Entry(2); e.Locked {
		t.Fatal("day 2 still locked")
	}
	if rec := post(t, h, "/week/approve", weekForm(nil)); rec.Code != http.StatusSeeOther {
		t.Fatalf("approve: %d", rec.Code)
	}
	p, _ := st.GetPlan(t.Context(), _w40)
	if p.Status != weekplan.StatusApproved || p.ApprovedBy != "sub-anna" {
		t.Fatalf("plan = %+v", p)
	}
	for _, path := range []string{"/week/lock", "/week/generate", "/week/context"} {
		if rec := post(t, h, path, weekForm(url.Values{"day": {"1"}, "locked": {"1"}})); rec.Code != http.StatusConflict {
			t.Errorf("%s on approved week: %d", path, rec.Code)
		}
	}
	_, body = get(t, h, "/week?y=2026&w=40")
	if !strings.Contains(body, "Godkänd") || strings.Contains(body, "Lås middagen") {
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

// Conditions cannot change under a running plan; the form is hidden and a post is refused.
func TestConditionsAreLockedWhilePlanning(t *testing.T) {
	st := newFakeStore()
	pl := &fakePlanner{st: st, gate: make(chan struct{})}
	h, s := newPlanningServer(t, i18n.SV, st, pl)
	post(t, h, "/week/generate", weekForm(nil))
	if _, body := get(t, h, "/week?y=2026&w=40"); strings.Contains(body, "Spara förutsättningar") {
		t.Error("conditions form shown while planning")
	}
	if rec := post(t, h, "/week/context", weekForm(url.Values{"days.0.home": {"on"}})); rec.Code != http.StatusConflict {
		t.Errorf("context while planning: %d, want 409", rec.Code)
	}
	close(pl.gate)
	s.jobs.wait()
}

// When the planner is busy with other weeks, the page says so instead of doing nothing.
func TestBusyPlannerSaysSo(t *testing.T) {
	st := newFakeStore()
	h, s := newPlanningServer(t, i18n.SV, st, &fakePlanner{st: st})
	release := make(chan struct{})
	for _, w := range []int{41, 42} {
		s.jobs.start(weekplan.Key{Year: 2026, Week: w}, func(context.Context) error {
			<-release
			return nil
		})
	}
	rec := post(t, h, "/week/generate", weekForm(nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "planeras just nu") {
		t.Errorf("busy: %d", rec.Code)
	}
	close(release)
	s.jobs.wait()
}

// With every planned dinner locked there is nothing left to plan, and the button says so.
func TestEveryDinnerLocked(t *testing.T) {
	st := newFakeStore()
	pl := &fakePlanner{st: st}
	h, s := newPlanningServer(t, i18n.SV, st, pl)
	post(t, h, "/week/generate", weekForm(nil))
	s.jobs.wait()
	c := weekplan.DefaultContext(7)
	for i := 2; i < 7; i++ { // the fake plans days 1 and 2
		c.Days[i].Skip = true
	}
	if err := st.SaveContext(t.Context(), _w40, c, [7]int{}); err != nil {
		t.Fatal(err)
	}
	for _, day := range []string{"1", "2"} {
		post(t, h, "/week/lock", weekForm(url.Values{"day": {day}, "locked": {"1"}}))
	}
	_, body := get(t, h, "/week?y=2026&w=40")
	if !strings.Contains(body, "Alla middagar är låsta") || !strings.Contains(body, "disabled") ||
		strings.Contains(body, "Planera om") {
		t.Fatalf("all locked:\n%s", body)
	}
}

// Review focus: the "use up first" lines are saved tidy, shown again, and limited.
func TestSaveUseUpFirst(t *testing.T) {
	st := newFakeStore()
	h, _ := newPlanningServer(t, i18n.SV, st, &fakePlanner{st: st})
	rec := post(t, h, "/week/context", weekForm(url.Values{"days.0.home": {"on"},
		"use_up": {" halv grädde \r\n\r\nris, kokt\nHalv grädde"}}))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d body %.200s", rec.Code, rec.Body.String())
	}
	p, _ := st.GetPlan(t.Context(), _w40)
	if strings.Join(p.Context.UseUp, "|") != "halv grädde|ris, kokt" {
		t.Fatalf("use up = %q", p.Context.UseUp)
	}
	if _, body := get(t, h, "/week?y=2026&w=40"); !strings.Contains(body, "Använd först") ||
		!strings.Contains(body, ">halv grädde\nris, kokt</textarea>") {
		t.Fatalf("not shown again: %.300s", body)
	}
	many := strings.Repeat("sak\n", 1)
	for i := range 11 {
		many += "sak " + itoa(int64(i)) + "\n"
	}
	rec = post(t, h, "/week/context", weekForm(url.Values{"days.0.home": {"on"}, "use_up": {many}}))
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Högst 10 saker") ||
		!strings.Contains(rec.Body.String(), "sak 10") {
		t.Fatalf("over the limit: %d %.300s", rec.Code, rec.Body.String())
	}
}

// Review focus: guests added after planning change the dinner's servings, so the list and
// the recipe scale follow; days that did not change are recomputed the same way.
func TestConditionsUpdatePlannedServings(t *testing.T) {
	st := newFakeStore()
	_, _ = st.CreateMember(t.Context(), household.Member{Name: "Anna", BirthYear: 1985})
	h, s := newPlanningServer(t, i18n.SV, st, &fakePlanner{st: st})
	post(t, h, "/week/generate", weekForm(nil))
	s.jobs.wait()
	rec := post(t, h, "/week/context", weekForm(url.Values{
		"days.0.home": {"on"}, "days.0.guests": {"3"}, "days.1.home": {"on"}}))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status %d body %.200s", rec.Code, rec.Body.String())
	}
	p, _ := st.GetPlan(t.Context(), _w40)
	if len(p.Entries) != 2 || p.Entries[0].Servings != 4 || p.Entries[1].Servings != 1 {
		t.Fatalf("entries = %+v", p.Entries)
	}
}

// Planning the unlocked days again shows which dinners are being replaced and which are kept.
func TestReplanningMarksUnlockedDays(t *testing.T) {
	st := newFakeStore()
	pl := &fakePlanner{st: st}
	h, s := newPlanningServer(t, i18n.SV, st, pl)
	post(t, h, "/week/generate", weekForm(nil))
	s.jobs.wait()
	post(t, h, "/week/lock", weekForm(url.Values{"day": {"2"}, "locked": {"1"}}))
	pl.gate = make(chan struct{})
	post(t, h, "/week/generate", weekForm(nil))
	_, body := get(t, h, "/week?y=2026&w=40")
	close(pl.gate)
	s.jobs.wait()
	if !strings.Contains(body, `<span class="text-sm text-muted line-through">Pumpasoppa</span>`) {
		t.Error("the unlocked dinner is not shown as being replaced")
	}
	if strings.Contains(body, `line-through">Köttbullar`) || !strings.Contains(body, `aria-label="Behålls"`) {
		t.Error("the locked dinner is not shown as kept")
	}
	// Days 1 and 3-7 are planned again (the default context plans all seven).
	if n := strings.Count(body, "Planeras…"); n != 6 {
		t.Errorf("%d days marked as being planned, want 6", n)
	}
	if _, body := get(t, h, "/week?y=2026&w=40"); strings.Contains(body, "Planeras…") {
		t.Error("still marked after planning")
	}
}

// Who is away on a day is kept with the week and shown checked on the next visit.
func TestAwayMembersAreKept(t *testing.T) {
	st := newFakeStore()
	id, _ := st.CreateMember(t.Context(), household.Member{Name: "Erik", BirthYear: 1984})
	h, _ := newPlanningServer(t, i18n.SV, st, &fakePlanner{st: st})
	rec := post(t, h, "/week/context", weekForm(url.Values{"days.0.home": {"on"},
		"days.2.home": {"on"}, "days.2.away": {itoa(id)}}))
	_, body := get(t, h, rec.Header().Get("Location"))
	checked := fmt.Sprintf(`id="days.2.away.%d" name="days.2.away" value="%d" checked`, id, id)
	if !strings.Contains(body, checked) {
		t.Errorf("Erik not shown away on Wednesday")
	}
	if strings.Contains(body, fmt.Sprintf(`id="days.0.away.%d" name="days.0.away" value="%d" checked`, id, id)) {
		t.Errorf("Erik shown away on Monday")
	}
}

// A day without dinner at home is a short row with only a strip of the day's colour.
func TestNoDinnerDayLooksOff(t *testing.T) {
	st := newFakeStore()
	h, _ := newPlanningServer(t, i18n.SV, st, &fakePlanner{st: st})
	post(t, h, "/week/context", weekForm(url.Values{"days.0.home": {"on"}}))
	_, body := get(t, h, "/week?y=2026&w=40")
	if n := strings.Count(body, "border-l-6 pl-3 text-muted day-strip-"); n != 6 {
		t.Errorf("%d days without dinner, want 6", n)
	}
	if !strings.Contains(body, `day-1"`) || strings.Contains(body, "day-strip-1") {
		t.Error("Monday, with dinner at home, is not a full row")
	}
}
