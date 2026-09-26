package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/jalet/matlistan/internal/i18n"
)

var _tokenRE = regexp.MustCompile(`mlt_[A-Za-z0-9_-]{43}`)

func exportWith(t *testing.T, h http.Handler, auth string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/api/v1/shopping-list/current.txt", nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Review focus 3: a key is shown once, works, and stops working when revoked.
func TestShortcutKeyLifecycle(t *testing.T) {
	h, st := approvedWeek(t)
	rec := post(t, h, "/settings/tokens", url.Values{"name": {"Annas iPhone"}})
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	body := rec.Body.String()
	key := _tokenRE.FindString(body)
	if key == "" || !strings.Contains(body, "https://matlistan.example.lan/api/v1/shopping-list/current.txt") {
		t.Fatalf("key or url not shown: %.400s", body)
	}
	if _, again := get(t, h, "/settings"); strings.Contains(again, key) {
		t.Fatal("key shown a second time")
	}
	res := exportWith(t, h, "Bearer "+key)
	if res.Code != http.StatusOK || !strings.HasPrefix(res.Header().Get("Content-Type"), "text/plain") ||
		!strings.Contains(res.Body.String(), "Matlistan vecka 40") ||
		!strings.Contains(res.Body.String(), "- 1,5 kg pumpa") {
		t.Fatalf("export: %d %q", res.Code, res.Body.String())
	}
	tokens, _ := st.ListAPITokens(t.Context())
	if len(tokens) != 1 || tokens[0].LastUsedAt == nil {
		t.Fatalf("tokens = %+v", tokens)
	}
	if rec := post(t, h, "/settings/tokens/"+itoa(tokens[0].ID)+"/delete", nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("revoke: %d", rec.Code)
	}
	if res := exportWith(t, h, "Bearer "+key); res.Code != http.StatusUnauthorized {
		t.Fatalf("revoked key: %d", res.Code)
	}
}

func TestExportRejectsBadKeys(t *testing.T) {
	h, _ := approvedWeek(t)
	for _, a := range []string{"", "Bearer nope", "Bearer mlt_" + strings.Repeat("A", 43), "Basic x"} {
		if res := exportWith(t, h, a); res.Code != http.StatusUnauthorized ||
			res.Body.Len() > 20 {
			t.Errorf("%q: %d %q", a, res.Code, res.Body.String())
		}
	}
}

// Review focus 4: with no approved week the export still says something useful.
func TestExportWithoutAList(t *testing.T) {
	st := newFakeStore()
	h := newServer(t, i18n.SV, true, st)
	rec := post(t, h, "/settings/tokens", url.Values{"name": {"iPad"}})
	key := _tokenRE.FindString(rec.Body.String())
	res := exportWith(t, h, "Bearer "+key)
	if res.Code != http.StatusOK || res.Body.String() != "Matlistan\n\nIngen godkänd vecka än.\n" {
		t.Fatalf("export: %d %q", res.Code, res.Body.String())
	}
}

func TestTokenNameIsRequired(t *testing.T) {
	h := newServer(t, i18n.SV, true, newFakeStore())
	if rec := post(t, h, "/settings/tokens", url.Values{"name": {" "}}); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blank name: %d", rec.Code)
	}
}
