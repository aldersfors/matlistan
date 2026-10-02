package web

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/weekplan"
)

func received(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	case <-time.After(50 * time.Millisecond):
		return false
	}
}

func TestBrokerCollapsesChangesPerWeek(t *testing.T) {
	b := newBroker()
	w40, cancel40 := b.subscribe(_w40)
	w41, cancel41 := b.subscribe(weekplan.Key{Year: 2026, Week: 41})
	defer cancel41()
	b.publish(_w40)
	b.publish(_w40) // does not block on the pending one
	if !received(w40) {
		t.Fatal("week 40 not told")
	}
	if received(w40) {
		t.Fatal("two changes not collapsed into one")
	}
	if received(w41) {
		t.Fatal("week 41 told about week 40")
	}
	cancel40()
	b.publish(_w40)
	if received(w40) {
		t.Fatal("told after cancel")
	}
	if _, ok := b.subs[_w40]; ok {
		t.Fatal("empty week kept")
	}
}

// liveServer serves h over a real connection with timeouts far below the stream's life.
func liveServer(t *testing.T) (*httptest.Server, *server) {
	t.Helper()
	st := newFakeStore()
	h, s := newPlanningServer(t, i18n.SV, st, &fakePlanner{st: st})
	s.heartbeat = 50 * time.Millisecond
	srv := httptest.NewUnstartedServer(h)
	srv.Config.ReadTimeout = 200 * time.Millisecond
	srv.Config.WriteTimeout = 200 * time.Millisecond
	srv.Start()
	t.Cleanup(srv.Close)
	return srv, s
}

// events opens the week's stream and returns its lines.
func events(t *testing.T, srv *httptest.Server) (<-chan string, func()) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
		srv.URL+"/week/events?y=2026&w=40", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("content type %q", ct)
	}
	lines := make(chan string)
	go func() {
		defer close(lines)
		sc := bufio.NewScanner(res.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	return lines, func() { _ = res.Body.Close() }
}

func waitFor(t *testing.T, lines <-chan string, want string) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case l, ok := <-lines:
			if !ok {
				t.Fatalf("stream ended before %q", want)
			}
			if l == want {
				return
			}
		case <-deadline:
			t.Fatalf("no %q", want)
		}
	}
}

// A saved change reaches an open page, even after the server's read and write timeouts.
func TestWeekEventsOutliveServerTimeouts(t *testing.T) {
	srv, s := liveServer(t)
	lines, closeStream := events(t, srv)
	defer closeStream()
	waitFor(t, lines, "retry: 5000")
	time.Sleep(500 * time.Millisecond) // past both timeouts
	waitFor(t, lines, ": ping")
	if rec := post(t, s.handler(), "/week/context", weekForm(url.Values{"days.0.home": {"on"}})); rec.Code != http.StatusSeeOther {
		t.Fatalf("save: %d", rec.Code)
	}
	waitFor(t, lines, "event: week")
}

func TestWeekEventsEndOnShutdown(t *testing.T) {
	st := newFakeStore()
	done := make(chan struct{})
	_, s := newPlanningServer(t, i18n.SV, st, &fakePlanner{st: st})
	s.Done = done
	srv := httptest.NewServer(s.handler())
	defer srv.Close()
	lines, closeStream := events(t, srv)
	defer closeStream()
	waitFor(t, lines, "retry: 5000")
	close(done)
	for range lines { //nolint:revive // drain until the server ends the stream
	}
}

func TestWeekPageListensForChanges(t *testing.T) {
	st := newFakeStore()
	h, _ := newPlanningServer(t, i18n.SV, st, &fakePlanner{st: st})
	_, body := get(t, h, "/week?y=2026&w=40")
	for _, want := range []string{`data-live="/week/events?y=2026&amp;w=40"`,
		`<script src="/static/live.js" defer>`, `hx-post="/week/context" hx-trigger="change"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s", want)
		}
	}
}

// Conditions save as they change, so the redirect keeps the form open; a plain visit does not.
func TestSavedConditionsStayOpen(t *testing.T) {
	st := newFakeStore()
	h, _ := newPlanningServer(t, i18n.SV, st, &fakePlanner{st: st})
	rec := post(t, h, "/week/context", weekForm(url.Values{"days.0.home": {"on"}}))
	loc := rec.Header().Get("Location")
	if loc != "/week?y=2026&w=40&conditions=open" {
		t.Fatalf("location %q", loc)
	}
	const open = `<details class="rounded-2xl bg-panel p-4" open>`
	if _, body := get(t, h, loc); !strings.Contains(body, open) {
		t.Error("conditions closed after a save")
	}
	if _, body := get(t, h, "/week?y=2026&w=40"); strings.Contains(body, open) {
		t.Error("conditions open on a plain visit")
	}
}
