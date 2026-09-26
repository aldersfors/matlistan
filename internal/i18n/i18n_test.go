package i18n

import (
	"context"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func mustLoad(t *testing.T, l Locale) *Catalog {
	t.Helper()
	c, err := Load(l)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

var _ph = regexp.MustCompile(`\{[a-z_]+\}`)

func placeholders(m Message) []string {
	p := append(_ph.FindAllString(m.One, -1), _ph.FindAllString(m.Other, -1)...)
	slices.Sort(p)
	return slices.Compact(p)
}

// Review focus 1: a key added to one catalog only must fail here, naming key and locale.
func TestCatalogsAgree(t *testing.T) {
	en := mustLoad(t, EN)
	for _, l := range Supported[1:] {
		c := mustLoad(t, l)
		if d := cmp.Diff(en.Keys(), c.Keys()); d != "" {
			t.Errorf("%s keys differ from en (-en +%s):\n%s", l, l, d)
		}
		for _, k := range en.Keys() {
			a, _ := en.Message(k)
			b, ok := c.Message(k)
			if !ok {
				continue
			}
			if a.Plural() != b.Plural() {
				t.Errorf("%s %q: plural form differs from en", l, k)
			}
			if d := cmp.Diff(placeholders(a), placeholders(b)); d != "" {
				t.Errorf("%s %q: placeholders differ from en:\n%s", l, k, d)
			}
		}
	}
}

func TestT(t *testing.T) {
	en, sv := mustLoad(t, EN), mustLoad(t, SV)
	if got := en.T("week.label", "n", 40); got != "Week 40" {
		t.Errorf("en = %q", got)
	}
	if got := sv.T("week.label", "n", 40); got != "Vecka 40" {
		t.Errorf("sv = %q", got)
	}
	if got := en.T("no.such.key"); got != "[no.such.key]" {
		t.Errorf("missing = %q", got)
	}
	if got := en.T("week.label"); got != "Week {n}" {
		t.Errorf("unfilled placeholder = %q, want it left as is", got)
	}
}

func TestN(t *testing.T) {
	sv := mustLoad(t, SV)
	for n, want := range map[int]string{0: "0 middagar", 1: "1 middag", 7: "7 middagar"} {
		if got := sv.N("week.meals", n); got != want {
			t.Errorf("N(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestOddArgsPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "name, value pairs") {
			t.Fatalf("recover = %v", r)
		}
	}()
	mustLoad(t, EN).T("week.label", "n")
}

func TestParseLocale(t *testing.T) {
	for in, want := range map[string]Locale{"": EN, "en": EN, "sv": SV} {
		got, err := ParseLocale(in)
		if err != nil || got != want {
			t.Errorf("ParseLocale(%q) = %q, %v", in, got, err)
		}
	}
	if _, err := ParseLocale("de"); err == nil || !strings.Contains(err.Error(), "en, sv") {
		t.Errorf("de: err = %v", err)
	}
}

func TestParseRejectsHalfPlural(t *testing.T) {
	_, err := parse([]byte(`{"a": {"one": "x"}, "b": {"other": "y"}}`))
	if err == nil || !strings.Contains(err.Error(), `"a"`) || !strings.Contains(err.Error(), `"b"`) {
		t.Fatalf("err = %v, want both keys named", err)
	}
}

func TestContext(t *testing.T) {
	ctx := WithCatalog(context.Background(), mustLoad(t, SV))
	if got := T(ctx, "nav.week"); got != "Veckan" {
		t.Errorf("T = %q", got)
	}
	if got := N(ctx, "week.meals", 1); got != "1 middag" {
		t.Errorf("N = %q", got)
	}
}
