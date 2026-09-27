package shopping

import (
	"strings"
	"testing"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/weekplan"
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

// Notes keeps headings, bullets and bold from pasted rich text, so the list reads like one.
func TestHTML(t *testing.T) {
	sv := catalog(t, i18n.SV)
	l := &List{Key: weekplan.Key{Year: 2026, Week: 40}, Items: []Item{
		{Name: "pumpa", Section: "produce", Quantity: 1.5, Unit: "kg"},
		{Name: "gul lök", Section: "produce", Quantity: 3, Unit: "pcs", Checked: true},
		{Name: "koriander", Section: "produce", Quantity: 1, Unit: "pcs", Optional: true},
		{Name: "salt", Section: "pantry"},
		{Name: "<b>tandkräm</b> & tvål", Section: "other", Manual: true},
	}}
	want := `<!doctype html><html lang="sv"><head><meta charset="utf-8">` +
		`<title>Matlistan vecka 40</title></head><body><h1>Matlistan vecka 40</h1>` +
		`<h2>Frukt och grönt</h2><ul><li><b>1,5 kg</b> pumpa</li>` +
		`<li><b>1 st</b> koriander <i>(valfri)</i></li></ul>` +
		`<h2>Skafferi</h2><ul><li>salt, efter smak</li></ul>` +
		`<h2>Övrigt</h2><ul><li>&lt;b&gt;tandkräm&lt;/b&gt; &amp; tvål</li></ul></body></html>`
	if got := HTML(sv, l); got != want {
		t.Fatalf("HTML =\n%s\nwant\n%s", got, want)
	}
	if got := HTML(sv, nil); !strings.Contains(got, "<h1>Matlistan</h1><p>Ingen godkänd vecka än.</p>") {
		t.Fatalf("nil list = %s", got)
	}
	bought := &List{Key: weekplan.Key{Year: 2026, Week: 40}, Items: []Item{{Name: "mjölk", Checked: true}}}
	if got := HTML(sv, bought); !strings.Contains(got, "<p>Allt är köpt.</p>") {
		t.Fatalf("bought list = %s", got)
	}
}

// The Shortcut turns each section's lines into Notes tickboxes, so items are plain lines.
func TestJSON(t *testing.T) {
	sv := catalog(t, i18n.SV)
	l := &List{Key: weekplan.Key{Year: 2026, Week: 40}, Items: []Item{
		{Name: "pumpa", Section: "produce", Quantity: 1.5, Unit: "kg"},
		{Name: "gul lök", Section: "produce", Quantity: 3, Unit: "pcs", Checked: true},
		{Name: "koriander", Section: "produce", Quantity: 1, Unit: "pcs", Optional: true},
		{Name: "<b>tandkräm</b>\nrad två", Section: "other", Manual: true},
	}}
	want := `{"title":"Matlistan vecka 40","sections":[` +
		`{"name":"Frukt och grönt","items":["1,5 kg pumpa","1 st koriander (valfri)"]},` +
		`{"name":"Övrigt","items":["<b>tandkräm</b> rad två"]}]}`
	if got := JSON(sv, l); got != want {
		t.Fatalf("JSON =\n%s\nwant\n%s", got, want)
	}
	if got := JSON(sv, nil); got != `{"title":"Matlistan","sections":[],"note":"Ingen godkänd vecka än."}` {
		t.Fatalf("nil list = %s", got)
	}
	bought := &List{Key: weekplan.Key{Year: 2026, Week: 40}, Items: []Item{{Name: "mjölk", Checked: true}}}
	if got := JSON(sv, bought); got != `{"title":"Matlistan vecka 40","sections":[],"note":"Allt är köpt."}` {
		t.Fatalf("bought list = %s", got)
	}
}

// Notes reads "- [ ]" as a tickbox when a Shortcut creates the note from Markdown.
func TestMarkdown(t *testing.T) {
	sv := catalog(t, i18n.SV)
	l := &List{Key: weekplan.Key{Year: 2026, Week: 40}, Items: []Item{
		{Name: "pumpa", Section: "produce", Quantity: 1.5, Unit: "kg"},
		{Name: "gul lök", Section: "produce", Quantity: 3, Unit: "pcs", Checked: true},
		{Name: "koriander", Section: "produce", Quantity: 1, Unit: "pcs", Optional: true},
		{Name: "salt", Section: "pantry"},
		{Name: "*tandkräm* [x]\n# rad två", Section: "other", Manual: true},
	}}
	want := "# Matlistan vecka 40\n\n" +
		"## Frukt och grönt\n- [ ] 1,5 kg pumpa\n- [ ] 1 st koriander _(valfri)_\n\n" +
		"## Skafferi\n- [ ] salt, efter smak\n\n" +
		"## Övrigt\n- [ ] \\*tandkräm\\* \\[x\\] \\# rad två\n"
	if got := Markdown(sv, l); got != want {
		t.Fatalf("Markdown =\n%q\nwant\n%q", got, want)
	}
	if got := Markdown(sv, nil); got != "# Matlistan\n\nIngen godkänd vecka än.\n" {
		t.Fatalf("nil list = %q", got)
	}
	bought := &List{Key: weekplan.Key{Year: 2026, Week: 40}, Items: []Item{{Name: "mjölk", Checked: true}}}
	if got := Markdown(sv, bought); got != "# Matlistan vecka 40\n\nAllt är köpt.\n" {
		t.Fatalf("bought list = %q", got)
	}
}
