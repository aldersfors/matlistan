package web

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
)

func leo() url.Values {
	return url.Values{"name": {"Leo"}, "birth_year": {"2014"}, "allergens": {"nuts", "nuts"},
		"diets": {"vegetarian"}, "likes": {"tacos"}}
}

func TestFamilyEmptyState(t *testing.T) {
	_, body := get(t, newServer(t, i18n.SV, true, newFakeStore()), "/family")
	if !strings.Contains(body, "Inga familjemedlemmar än") {
		t.Fatalf("body lacks the empty state")
	}
}

func TestAddMember(t *testing.T) {
	st := newFakeStore()
	h := newServer(t, i18n.SV, true, st)
	rec := post(t, h, "/family", leo())
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/family" {
		t.Fatalf("status %d location %q", rec.Code, rec.Header().Get("Location"))
	}
	ms, _ := st.ListMembers(t.Context())
	if len(ms) != 1 || len(ms[0].Allergens) != 1 {
		t.Fatalf("stored %+v", ms)
	}
	_, body := get(t, h, "/family")
	for _, want := range []string{"Leo", "12 år", "Nötter", "Vegetarisk", "Det här är jag",
		"Välj vilken familjemedlem du är"} {
		if !strings.Contains(body, want) {
			t.Errorf("family page lacks %q", want)
		}
	}
}

func TestAddMemberShowsErrorsAndKeepsInput(t *testing.T) {
	h := newServer(t, i18n.SV, true, newFakeStore())
	rec := post(t, h, "/family", url.Values{"name": {""}, "birth_year": {"tolv"},
		"likes": {"pannkakor"}, "allergens": {"cats"}})
	body := rec.Body.String()
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status %d", rec.Code)
	}
	for _, want := range []string{"Obligatoriskt", "Inte ett tal", "Inget giltigt val",
		"pannkakor", `value="tolv"`, "Rätta de markerade fälten"} {
		if !strings.Contains(body, want) {
			t.Errorf("form lacks %q", want)
		}
	}
}

func TestMemberNameIsEscaped(t *testing.T) {
	st := newFakeStore()
	h := newServer(t, i18n.EN, true, st)
	v := leo()
	v.Set("name", "<script>alert(1)</script>")
	post(t, h, "/family", v)
	_, body := get(t, h, "/family")
	if strings.Contains(body, "<script>alert(1)") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("member name is not escaped")
	}
}

func TestEditMember(t *testing.T) {
	st := newFakeStore()
	id, _ := st.CreateMember(t.Context(), household.Member{Name: "Leo", BirthYear: 2014})
	h := newServer(t, i18n.EN, true, st)
	_, body := get(t, h, "/family/1/edit")
	if !strings.Contains(body, `value="Leo"`) || !strings.Contains(body, "Edit Leo") {
		t.Fatalf("edit form: %.200s", body)
	}
	v := leo()
	v.Set("name", "Leonard")
	if rec := post(t, h, "/family/1", v); rec.Code != http.StatusSeeOther {
		t.Fatalf("update status %d", rec.Code)
	}
	if m, _ := st.GetMember(t.Context(), id); m.Name != "Leonard" {
		t.Fatalf("name = %q", m.Name)
	}
}

// Review focus 5: unknown and malformed ids are 404.
func TestUnknownMember(t *testing.T) {
	h := newServer(t, i18n.EN, true, newFakeStore())
	for _, p := range []string{"/family/99/edit", "/family/abc/edit", "/family/-1/edit"} {
		if res, _ := get(t, h, p); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s: %d", p, res.StatusCode)
		}
	}
	for _, p := range []string{"/family/99", "/family/99/archive", "/family/99/me"} {
		if rec := post(t, h, p, leo()); rec.Code != http.StatusNotFound {
			t.Errorf("POST %s: %d", p, rec.Code)
		}
	}
}

// Review focus 4: linking, then removing the linked member.
func TestThisIsMeAndRemove(t *testing.T) {
	st := newFakeStore()
	id, _ := st.CreateMember(t.Context(), household.Member{Name: "Anna", BirthYear: 1985})
	h := newServer(t, i18n.SV, true, st)
	if rec := post(t, h, "/family/1/me", nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("me status %d", rec.Code)
	}
	if m, _ := st.GetMember(t.Context(), id); m.Subject != "sub-anna" {
		t.Fatalf("subject = %q", m.Subject)
	}
	_, body := get(t, h, "/family")
	if !strings.Contains(body, ">Du<") || strings.Contains(body, "Välj vilken familjemedlem") {
		t.Fatal("linked member not marked, or hint still shown")
	}
	if rec := post(t, h, "/family/1/archive", nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("archive status %d", rec.Code)
	}
	if res, _ := get(t, h, "/family"); res.StatusCode != http.StatusOK {
		t.Fatalf("family after removal: %d", res.StatusCode)
	}
}
