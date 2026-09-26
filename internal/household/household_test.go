package household

import (
	"slices"
	"testing"
	"time"

	"github.com/jalet/matlistan/internal/i18n"
)

var _now = time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)

func TestMemberValidate(t *testing.T) {
	ok := Member{Name: "Leo", BirthYear: 2014, Allergens: []string{"nuts"}}
	if e := ok.Validate(_now); len(e) != 0 {
		t.Fatalf("valid member: %v", e)
	}
	bad := Member{Name: "", BirthYear: 1850, Diets: []string{"keto"},
		Allergens: []string{"cats"}, Likes: string(make([]rune, 501))}
	e := bad.Validate(_now)
	for field, key := range map[string]string{"name": "form.required",
		"birth_year": "form.out_of_range", "diets": "form.unknown_option",
		"allergens": "form.unknown_option", "likes": "form.too_long"} {
		if e[field] != key {
			t.Errorf("%s = %q, want %q", field, e[field], key)
		}
	}
	if future := (Member{Name: "X", BirthYear: 2027}).Validate(_now); future["birth_year"] == "" {
		t.Error("a birth year after this year is accepted")
	}
}

// Review focus 3: duplicate choices and stray whitespace are removed before storing.
func TestMemberClean(t *testing.T) {
	m := Member{Name: "  Maja ", Likes: " tacos ", Allergens: []string{"nuts", "milk", "nuts"},
		Diets: nil}.Clean()
	if m.Name != "Maja" || m.Likes != "tacos" {
		t.Errorf("trim: %q %q", m.Name, m.Likes)
	}
	if !slices.Equal(m.Allergens, []string{"milk", "nuts"}) {
		t.Errorf("allergens = %v", m.Allergens)
	}
	if m.Diets == nil {
		t.Error("nil diets stay nil; want an empty slice so the database column is never NULL")
	}
}

func TestAgeAndPortion(t *testing.T) {
	cases := []struct {
		year   int
		age    int
		factor float64
	}{{2025, 1, 0.3}, {2023, 3, 0.3}, {2022, 4, 0.5}, {2018, 8, 0.5}, {2017, 9, 0.75},
		{2014, 12, 0.75}, {2013, 13, 1}, {1980, 46, 1}, {2030, 0, 0.3}}
	for _, c := range cases {
		a := Age(c.year, _now)
		if a != c.age || PortionFactor(a) != c.factor {
			t.Errorf("born %d: age %d factor %v, want %d %v", c.year, a, PortionFactor(a),
				c.age, c.factor)
		}
	}
}

func TestSettingsValidate(t *testing.T) {
	if e := DefaultSettings().Validate(); len(e) != 0 {
		t.Fatalf("defaults invalid: %v", e)
	}
	e := Settings{DinnersPerWeek: 8, WeeknightMinutes: 5, RepeatWindowWeeks: 0,
		LibraryShare: 101}.Validate()
	for _, f := range []string{"dinners_per_week", "weeknight_minutes", "repeat_window_weeks",
		"library_share"} {
		if e[f] != "form.out_of_range" {
			t.Errorf("%s = %q", f, e[f])
		}
	}
}

func TestStapleKey(t *testing.T) {
	if StapleName("  Havre   gryn ") != "Havre gryn" || StapleKey(" SALT ") != "salt" ||
		StapleKey("Ärtor") != "ärtor" {
		t.Error("staple normalisation")
	}
}

func TestCatalogsNameEveryOption(t *testing.T) {
	for _, l := range i18n.Supported {
		c, err := i18n.Load(l)
		if err != nil {
			t.Fatal(err)
		}
		for _, a := range Allergens {
			if _, ok := c.Message("allergen." + a); !ok {
				t.Errorf("%s lacks allergen.%s", l, a)
			}
		}
		for _, d := range Diets {
			if _, ok := c.Message("diet." + d); !ok {
				t.Errorf("%s lacks diet.%s", l, d)
			}
		}
	}
}
