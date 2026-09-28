package shopping

import (
	"slices"
	"testing"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/recipes"
)

func use(day, servings, base int, name string, q float64, unit, section string) Use {
	return Use{Day: day, Servings: servings, RecipeServings: base,
		Ingredient: recipes.Ingredient{Name: name, Quantity: q, Unit: unit, Section: section}}
}

func find(t *testing.T, items []Item, name, unit string) Item {
	t.Helper()
	for _, it := range items {
		if it.Name == name && it.Unit == unit {
			return it
		}
	}
	t.Fatalf("no %q in %q among %+v", name, unit, items)
	return Item{}
}

// Review focus 1: units merge within a dimension only.
func TestBuildMergesUnits(t *testing.T) {
	items, _ := Build([]Use{
		use(1, 4, 4, "vispgrädde", 1, "dl", "dairy"),
		use(3, 4, 4, "Vispgrädde ", 2, "tbsp", "dairy"),
		use(1, 4, 4, "blandfärs", 500, "g", "meat_fish"),
		use(2, 8, 4, "blandfärs", 0.5, "kg", "meat_fish"), // scaled to 1 kg
		use(1, 4, 4, "gul lök", 2, "pcs", "produce"),
		use(2, 6, 4, "gul lök", 1, "pcs", "produce"), // 1.5 -> total 3.5 -> 4
		use(3, 4, 4, "gul lök", 100, "g", "produce"),
	}, nil)
	cream := find(t, items, "vispgrädde", "dl")
	if cream.Quantity != 1.3 || !slices.Equal(cream.Days, []int{1, 3}) {
		t.Errorf("cream = %+v, want 1.3 dl on days 1 and 3", cream)
	}
	if mince := find(t, items, "blandfärs", "kg"); mince.Quantity != 1.5 {
		t.Errorf("mince = %+v", mince)
	}
	if onions := find(t, items, "gul lök", "pcs"); onions.Quantity != 4 {
		t.Errorf("onions = %+v, want 4 pcs (rounded up)", onions)
	}
	if grams := find(t, items, "gul lök", "g"); grams.Quantity != 100 {
		t.Errorf("onion grams = %+v", grams)
	}
}

func TestBuildKeepsOneUnitWhenAllAgree(t *testing.T) {
	items, _ := Build([]Use{use(1, 4, 4, "olja", 2, "tbsp", "pantry"),
		use(2, 4, 4, "olja", 1, "tbsp", "pantry")}, nil)
	if got := find(t, items, "olja", "tbsp"); got.Quantity != 3 {
		t.Fatalf("olja = %+v", got)
	}
}

func TestBuildToTaste(t *testing.T) {
	items, _ := Build([]Use{use(1, 4, 4, "salt", 0, "", "pantry"),
		use(2, 4, 4, "salt", 1, "tsp", "pantry"), use(3, 4, 4, "svartpeppar", 0, "", "pantry")}, nil)
	if len(items) != 2 {
		t.Fatalf("items = %+v", items)
	}
	if find(t, items, "salt", "tsp").Quantity != 1 || find(t, items, "svartpeppar", "").Quantity != 0 {
		t.Fatal("to-taste handling")
	}
}

// Review focus 2: staples match however they are typed.
func TestBuildLeavesOutStaples(t *testing.T) {
	items, excluded := Build([]Use{use(1, 4, 4, "salt ", 1, "tsp", "pantry"),
		use(2, 4, 4, "Salt", 0, "", "pantry"), use(1, 4, 4, "pumpa", 1, "kg", "produce")},
		[]household.Staple{{Name: "SALT"}})
	if len(items) != 1 || items[0].Name != "pumpa" || excluded != 1 {
		t.Fatalf("items = %+v excluded = %d", items, excluded)
	}
}

func TestBuildOrdersBySectionThenName(t *testing.T) {
	items, _ := Build([]Use{use(1, 4, 4, "vispgrädde", 1, "dl", "dairy"),
		use(1, 4, 4, "zucchini", 1, "pcs", "produce"), use(1, 4, 4, "aubergine", 1, "pcs", "produce")}, nil)
	var names []string
	for _, it := range items {
		names = append(names, it.Name)
	}
	if !slices.Equal(names, []string{"aubergine", "zucchini", "vispgrädde"}) {
		t.Fatalf("order = %v", names)
	}
}

func TestBuildOptionalOnlyWhenAlwaysOptional(t *testing.T) {
	a := use(1, 4, 4, "koriander", 1, "pcs", "produce")
	a.Ingredient.Optional = true
	b := use(2, 4, 4, "koriander", 1, "pcs", "produce")
	items, _ := Build([]Use{a, b}, nil)
	if items[0].Optional {
		t.Fatal("needed on day 2, so not optional")
	}
	items, _ = Build([]Use{a}, nil)
	if !items[0].Optional {
		t.Fatal("only optional uses")
	}
}

// A required "to taste" use on another day is not swallowed by an optional amount.
func TestToTasteUseKeepsItsDayAndNeed(t *testing.T) {
	a := use(1, 4, 4, "koriander", 1, "pcs", "produce")
	a.Ingredient.Optional = true
	items, _ := Build([]Use{a, use(2, 4, 4, "koriander", 0, "", "produce")}, nil)
	if len(items) != 1 || items[0].Optional || !slices.Equal(items[0].Days, []int{1, 2}) {
		t.Fatalf("items = %+v", items)
	}
}

// Amounts read in the natural unit, also when every use has the same unit.
func TestBuildShowsReadableUnits(t *testing.T) {
	items, _ := Build([]Use{
		use(1, 4, 4, "nötfärs", 800, "g", "meat_fish"), use(2, 4, 4, "nötfärs", 600, "g", "meat_fish"),
		use(1, 4, 4, "passerade tomater", 250, "ml", "pantry"),
		use(1, 4, 4, "ris", 8, "dl", "pantry"),
		use(1, 4, 4, "mjölk", 12, "dl", "dairy"),
		use(1, 4, 4, "ost", 900, "g", "dairy"),
		use(1, 4, 4, "smör", 3, "tbsp", "dairy"),
		use(1, 4, 4, "salt", 50, "ml", "pantry"),
	}, nil)
	for _, c := range []struct {
		name, unit string
		q          float64
	}{{"nötfärs", "kg", 1.4}, {"passerade tomater", "dl", 2.5}, {"ris", "dl", 8},
		{"mjölk", "l", 1.2}, {"ost", "g", 900}, {"smör", "tbsp", 3}, {"salt", "ml", 50}} {
		if got := find(t, items, c.name, c.unit); got.Quantity != c.q {
			t.Errorf("%s = %+v, want %v %s", c.name, got, c.q, c.unit)
		}
	}
}

// The same food under its singular and plural name is one line, shown in the singular.
func TestBuildMergesPluralNames(t *testing.T) {
	items, _ := Build([]Use{
		use(1, 4, 4, "morot", 2, "pcs", "produce"),
		use(2, 4, 4, "morötter", 10, "pcs", "produce"),
		use(3, 4, 4, "Tomater", 2, "pcs", "produce"),
		use(4, 4, 4, "tomat", 1, "pcs", "produce"),
		use(1, 4, 4, "mjöl", 2, "dl", "pantry"),
		use(1, 4, 4, "mjölk", 2, "dl", "dairy"),
	}, nil)
	if got := find(t, items, "morot", "pcs"); got.Quantity != 12 || !slices.Equal(got.Days, []int{1, 2}) {
		t.Errorf("morot = %+v", got)
	}
	if got := find(t, items, "tomat", "pcs"); got.Quantity != 3 {
		t.Errorf("tomat = %+v", got)
	}
	for _, it := range items {
		if it.Name == "morötter" || it.Name == "Tomater" {
			t.Errorf("plural line kept: %+v", it)
		}
	}
	find(t, items, "mjöl", "dl")
	find(t, items, "mjölk", "dl")
}

// A staple written in the plural still keeps its singular off the list, and the reverse.
func TestStaplesMatchPluralNames(t *testing.T) {
	items, n := Build([]Use{use(1, 4, 4, "vitlöksklyftor", 4, "pcs", "produce")},
		[]household.Staple{{Name: "Vitlöksklyfta"}})
	if len(items) != 0 || n != 1 {
		t.Fatalf("items %+v excluded %d", items, n)
	}
}

// A rebuilt line is ticked when a ticked planned line had the same base name, whatever its
// amount or plural; hand-added items are not matched here, they are kept as they are.
func TestCarryTicks(t *testing.T) {
	old := []Item{
		{Name: "morötter", Quantity: 10, Unit: "pcs", Checked: true},
		{Name: "gul lök", Quantity: 2, Unit: "pcs"},
		{Name: "Grädde", Quantity: 2, Unit: "dl", Checked: true},
		{Name: "mjöl", Checked: true, Manual: true},
	}
	built := []Item{
		{Name: "morot", Quantity: 12, Unit: "pcs"},
		{Name: "gul lök", Quantity: 2, Unit: "pcs"},
		{Name: "grädde", Quantity: 4, Unit: "dl"},
		{Name: "mjöl", Quantity: 1, Unit: "dl"},
	}
	got := CarryTicks(old, built)
	want := []bool{true, false, true, false}
	for i, it := range got {
		if it.Checked != want[i] {
			t.Errorf("%s checked = %v, want %v", it.Name, it.Checked, want[i])
		}
	}
}
