package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/aldersfors/matlistan/internal/i18n"
)

const _pageKey = "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"

func postJSON(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

const _goodSub = `{"endpoint":"https://web.push.apple.com/abc","keys":{"p256dh":"` + _pageKey + `","auth":"BTBZMqHH6r4Tts7J_aSIgg"}}`

func TestPushSectionHiddenWithoutAKey(t *testing.T) {
	h := newServer(t, i18n.SV, true, newFakeStore())
	if _, body := get(t, h, "/settings"); strings.Contains(body, "Aviseringar") {
		t.Fatal("push section without a key")
	}
	if rec := postJSON(t, h, "/settings/push", _goodSub); rec.Code != http.StatusNotFound {
		t.Fatalf("subscribe without a key: %d", rec.Code)
	}
}

func TestSubscribeAndUnsubscribe(t *testing.T) {
	st := newFakeStore()
	h := newPushServer(t, st, _pageKey)
	_, body := get(t, h, "/settings")
	if !strings.Contains(body, "Aviseringar") || !strings.Contains(body, `data-key="`+_pageKey+`"`) ||
		!strings.Contains(body, `src="/static/push.js"`) {
		t.Fatalf("section: %.500s", body)
	}
	if rec := postJSON(t, h, "/settings/push", _goodSub); rec.Code != http.StatusNoContent {
		t.Fatalf("subscribe: %d %s", rec.Code, rec.Body.String())
	}
	if n, _ := st.CountPushSubscriptions(t.Context()); n != 1 {
		t.Fatalf("count %d", n)
	}
	if _, body := get(t, h, "/settings"); !strings.Contains(body, "1 enhet") {
		t.Fatal("device count not shown")
	}
	if rec := postJSON(t, h, "/settings/push/delete", `{"endpoint":"https://web.push.apple.com/abc"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("delete: %d", rec.Code)
	}
	if rec := postJSON(t, h, "/settings/push/delete", `{"endpoint":"https://web.push.apple.com/abc"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("delete again: %d", rec.Code)
	}
}

func TestSubscribeRejectsBadInput(t *testing.T) {
	st := newFakeStore()
	h := newPushServer(t, st, _pageKey)
	for name, body := range map[string]string{
		"http":      strings.Replace(_goodSub, "https://web.push.apple.com", "http://web.push.apple.com", 1),
		"host":      strings.Replace(_goodSub, "web.push.apple.com", "evil.example", 1),
		"private":   strings.Replace(_goodSub, "web.push.apple.com", "10.0.0.1", 1),
		"short key": strings.Replace(_goodSub, _pageKey, "BAECAw", 1),
		"auth":      strings.Replace(_goodSub, "BTBZMqHH6r4Tts7J_aSIgg", "AAAA", 1),
		"not json":  "endpoint=x",
		"too big":   `{"endpoint":"` + strings.Repeat("a", 5000) + `"}`,
	} {
		if rec := postJSON(t, h, "/settings/push", body); rec.Code != http.StatusUnprocessableEntity &&
			rec.Code != http.StatusRequestEntityTooLarge {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if n, _ := st.CountPushSubscriptions(t.Context()); n != 0 {
		t.Fatalf("stored %d", n)
	}
}

func TestSubscribeAtTheLimit(t *testing.T) {
	st := newFakeStore()
	st.pushLimit = 0
	h := newPushServer(t, st, _pageKey)
	if rec := postJSON(t, h, "/settings/push", _goodSub); rec.Code != http.StatusConflict {
		t.Fatalf("at the limit: %d", rec.Code)
	}
}

func TestCrossSiteSubscribeIsRefused(t *testing.T) {
	h := newPushServer(t, newFakeStore(), _pageKey)
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/settings/push", strings.NewReader(_goodSub))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestServiceWorkerIsServed(t *testing.T) {
	h := newServer(t, i18n.SV, false, newFakeStore()) // signed out: the worker is public
	res, body := get(t, h, "/sw.js")
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/javascript") ||
		res.Header.Get("Cache-Control") != "no-cache" || !strings.Contains(body, "notificationclick") {
		t.Fatalf("sw.js: %d %q %q", res.StatusCode, res.Header.Get("Content-Type"), res.Header.Get("Cache-Control"))
	}
}

// newPushServer is newServer, signed in, with web push on.
func newPushServer(t *testing.T, st *fakeStore, key string) http.Handler {
	t.Helper()
	c, err := i18n.Load(i18n.SV)
	if err != nil {
		t.Fatal(err)
	}
	sthlm, _ := time.LoadLocation("Europe/Stockholm")
	return New(Deps{Catalog: c, Auth: fakeAuth{signedIn: true}, Store: st,
		Now:     func() time.Time { return time.Date(2026, 9, 27, 8, 0, 0, 0, sthlm) },
		BaseURL: "https://matlistan.example.lan", Log: zerolog.Nop(), PushKey: key})
}
