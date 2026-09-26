package web

import (
	"strings"
	"testing"

	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/recipes"
)

func cookStore(t *testing.T) *fakeStore {
	t.Helper()
	st := newFakeStore()
	_, _ = st.CreateRecipe(t.Context(), recipes.Recipe{Title: "Köttbullar", Lang: i18n.SV,
		Servings: 4, TotalMinutes: 40, Source: "manual",
		Steps: []string{"Blanda smeten.", "Koka potatisen i 20 minuter.", "Stek bullarna 10 min."},
		Ingredients: []recipes.Ingredient{{Name: "blandfärs", Quantity: 500, Unit: "g",
			Section: "meat_fish"}}})
	return st
}

func TestMinutesIn(t *testing.T) {
	for step, want := range map[string]int{"Koka i 20 minuter.": 20, "Stek 10 min": 10,
		"Bake for 35 minutes": 35, "Servera.": 0, "Rör om 1 minut": 0} {
		if got := minutesIn(step); got != want {
			t.Errorf("%q = %d, want %d", step, got, want)
		}
	}
}

// Review focus 5: servings from the address, bounded.
func TestRecipeServingsFromTheWeek(t *testing.T) {
	h := newServer(t, i18n.SV, true, cookStore(t))
	_, body := get(t, h, "/recipes/1?servings=6&day=5")
	if !strings.Contains(body, "750 g blandfärs") || !strings.Contains(body, "6 portioner") ||
		!strings.Contains(body, `href="/recipes/1/cook?servings=6&amp;day=5"`) {
		t.Fatalf("scaled recipe: %.400s", body)
	}
	for _, q := range []string{"servings=abc", "servings=0", "servings=99"} {
		if _, b := get(t, h, "/recipes/1?"+q); !strings.Contains(b, "500 g blandfärs") {
			t.Errorf("%s did not fall back to the recipe's servings", q)
		}
	}
}

// Review focus 4: every step is in the page; timers only where minutes are named, hidden
// until the script shows them.
func TestCookMode(t *testing.T) {
	h := newServer(t, i18n.SV, true, cookStore(t))
	_, body := get(t, h, "/recipes/1/cook?servings=6&day=5")
	for _, want := range []string{"Blanda smeten.", "Koka potatisen i 20 minuter.",
		"Stek bullarna 10 min.", `data-minutes="20"`, `data-minutes="10"`,
		"Starta timer 20 minuter", `src="/static/cook.js"`, "day-5", "750 g blandfärs",
		`data-template="Steg {n} av {total}"`, `id="cook-timers"`,
		`data-template="Steg {n}: {left}"`, `id="cook-announce" aria-live="polite"`} {
		if !strings.Contains(body, want) {
			t.Errorf("cook page lacks %q", want)
		}
	}
	if strings.Count(body, "data-minutes=") != 2 {
		t.Error("a timer on a step without minutes")
	}
	if !strings.Contains(body, `hidden data-minutes`) && !strings.Contains(body, `data-minutes="20" hidden`) {
		t.Error("timer buttons are not hidden before the script runs")
	}
}

func TestWeekLinksCarryServings(t *testing.T) {
	h, _ := approvedWeek(t)
	_, body := get(t, h, "/week?y=2026&w=40")
	if !strings.Contains(body, `href="/recipes/1?servings=4&amp;day=1"`) {
		t.Fatalf("week link: %.600s", body)
	}
}
