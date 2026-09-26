package shopping

import (
	"testing"

	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/weekplan"
)

func catalog(t *testing.T, l i18n.Locale) *i18n.Catalog {
	t.Helper()
	c, err := i18n.Load(l)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLine(t *testing.T) {
	sv := catalog(t, i18n.SV)
	for _, c := range []struct {
		name string
		q    float64
		unit string
		opt  bool
		want string
	}{{"vispgrädde", 1.5, "dl", false, "1,5 dl vispgrädde"},
		{"gul lök", 2, "pcs", false, "2 st gul lök"},
		{"salt", 0, "", false, "salt, efter smak"},
		{"koriander", 1, "pcs", true, "1 st koriander (valfri)"}} {
		if got := Line(sv, c.name, c.q, c.unit, c.opt); got != c.want {
			t.Errorf("Line = %q, want %q", got, c.want)
		}
	}
}

// Review focus 4: the export always has a title, even with nothing to show.
func TestText(t *testing.T) {
	sv := catalog(t, i18n.SV)
	l := &List{Key: weekplan.Key{Year: 2026, Week: 40}, Items: []Item{
		{Name: "pumpa", Section: "produce", Quantity: 1.5, Unit: "kg"},
		{Name: "gul lök", Section: "produce", Quantity: 3, Unit: "pcs", Checked: true},
		{Name: "vispgrädde", Section: "dairy", Quantity: 3, Unit: "dl"},
		{Name: "tandkräm", Section: "other", Manual: true},
	}}
	want := "Matlistan vecka 40\n\nFrukt och grönt\n- 1,5 kg pumpa\n\nMejeri\n- 3 dl vispgrädde\n\n" +
		"Övrigt\n- tandkräm\n"
	if got := Text(sv, l); got != want {
		t.Fatalf("Text =\n%q\nwant\n%q", got, want)
	}
	if got := Text(sv, nil); got != "Matlistan\n\nIngen godkänd vecka än.\n" {
		t.Fatalf("nil list = %q", got)
	}
	empty := &List{Key: weekplan.Key{Year: 2026, Week: 40}, Excluded: 3}
	if got := Text(sv, empty); got != "Matlistan vecka 40\n\nAllt för veckan finns redan hemma.\n" {
		t.Fatalf("empty list = %q", got)
	}
}

func TestTextWhenEverythingIsBought(t *testing.T) {
	sv := catalog(t, i18n.SV)
	l := &List{Key: weekplan.Key{Year: 2026, Week: 40}, Items: []Item{{Name: "pumpa",
		Section: "produce", Quantity: 1, Unit: "kg", Checked: true}}}
	if got := Text(sv, l); got != "Matlistan vecka 40\n\nAllt är köpt.\n" {
		t.Fatalf("Text = %q", got)
	}
}
