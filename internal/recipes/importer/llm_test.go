package importer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/planner"
)

type fakeLLM struct {
	system string
	schema map[string]any
	sent   []string
	reply  string
	err    error
}

func (f *fakeLLM) NewSession(system string, schema map[string]any) planner.Session {
	f.system, f.schema = system, schema
	return f
}

func (f *fakeLLM) Send(_ context.Context, text string) (planner.Reply, error) {
	f.sent = append(f.sent, text)
	return planner.Reply{Text: f.reply, Usage: planner.Usage{Input: 10, Output: 5}}, f.err
}

func TestNormaliserIngredients(t *testing.T) {
	f := &fakeLLM{reply: `{"ingredients":[{"name":"blandfärs","quantity":500,"unit":"g","section":"meat_fish","optional":false,"heading":false},{"name":"ägg","quantity":1,"unit":"pcs","section":"dairy","optional":false,"heading":false}]}`}
	got, notes, err := NewNormaliser(f).Ingredients(context.Background(), []string{"500 g blandfärs", "1 ägg"}, i18n.SV)
	if err != nil || len(got) != 2 || got[0].Quantity != 500 || got[0].Unit != "g" ||
		got[0].Section != "meat_fish" || len(notes) != 0 {
		t.Fatalf("got %+v %v %v", got, notes, err)
	}
	if !strings.Contains(f.system, "Swedish") || !strings.Contains(f.system, "data") ||
		!strings.Contains(f.sent[0], "500 g blandfärs") {
		t.Errorf("prompt: %q / %q", f.system, f.sent)
	}
	units := f.schema["properties"].(map[string]any)["ingredients"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)["unit"]
	if !strings.Contains(fmtAny(units), `"dl"`) {
		t.Errorf("schema lacks the unit enum: %v", units)
	}
}

// Review focus 2: an amount that cannot be kept never disappears: the raw line stays as the
// name, with a note.
func TestNormaliserClearsUnknownKeys(t *testing.T) {
	f := &fakeLLM{reply: `{"ingredients":[{"name":"mjölk","quantity":2,"unit":"cup","section":"drinks","optional":false,"heading":false}]}`}
	got, notes, err := NewNormaliser(f).Ingredients(context.Background(), []string{"2 cups milk"}, i18n.SV)
	if err != nil || got[0].Unit != "" || got[0].Section != "other" || got[0].Quantity != 0 ||
		got[0].Name != "2 cups milk" {
		t.Fatalf("got %+v %v", got, err)
	}
	if len(notes) == 0 || notes[0] != "import.note.unit_unknown" {
		t.Errorf("notes = %v", notes)
	}
}

// Review: a quantity with no unit ("1 burk hela tomater") keeps the whole line.
func TestNormaliserKeepsTheLineWhenTheUnitIsMissing(t *testing.T) {
	f := &fakeLLM{reply: `{"ingredients":[{"name":"hela tomater","quantity":1,"unit":null,"section":"pantry","optional":false,"heading":false}]}`}
	got, notes, _ := NewNormaliser(f).Ingredients(context.Background(),
		[]string{"1 burk hela tomater (à ca 400 g)"}, i18n.SV)
	if got[0].Name != "1 burk hela tomater (à ca 400 g)" || got[0].Quantity != 0 || len(notes) == 0 {
		t.Fatalf("got %+v notes %v", got, notes)
	}
}

// Review: a skipped or merged line would vanish; a count mismatch falls back to every raw line.
func TestNormaliserFallsBackWhenLinesGoMissing(t *testing.T) {
	f := &fakeLLM{reply: `{"ingredients":[{"name":"ägg","quantity":3,"unit":"pcs","section":"dairy","optional":false,"heading":false}]}`}
	got, notes, err := NewNormaliser(f).Ingredients(context.Background(),
		[]string{"3 ägg", "0,5 tsk salt"}, i18n.SV)
	if err != nil || len(got) != 2 || got[1].Name != "0,5 tsk salt" || got[0].Name != "3 ägg" {
		t.Fatalf("got %+v %v", got, err)
	}
	if len(notes) == 0 || notes[0] != "import.note.unparsed" {
		t.Errorf("notes = %v", notes)
	}
}

// Review: headings in recipeIngredient ("Till servering") are not ingredients.
func TestNormaliserDropsHeadings(t *testing.T) {
	f := &fakeLLM{reply: `{"ingredients":[{"name":"till servering","quantity":null,"unit":null,"section":"other","optional":false,"heading":true},{"name":"lingonsylt","quantity":null,"unit":null,"section":"pantry","optional":true,"heading":false}]}`}
	got, _, err := NewNormaliser(f).Ingredients(context.Background(),
		[]string{"Till servering", "lingonsylt"}, i18n.SV)
	if err != nil || len(got) != 1 || got[0].Name != "lingonsylt" {
		t.Fatalf("got %+v %v", got, err)
	}
	if !strings.Contains(f.system, "pcs") || !strings.Contains(f.system, "msk") ||
		!strings.Contains(f.system, "heading") {
		t.Errorf("prompt lacks unit mapping or heading rule: %q", f.system)
	}
}

func TestNormaliserErrors(t *testing.T) {
	for name, f := range map[string]*fakeLLM{
		"transport": {err: errors.New("boom")},
		"refused":   {err: planner.ErrRefused},
		"not json":  {reply: "Here you go!"},
	} {
		if _, _, err := NewNormaliser(f).Ingredients(context.Background(), []string{"x"}, i18n.SV); !errors.Is(err, ErrModelUnavailable) {
			t.Errorf("%s: err = %v", name, err)
		}
	}
}

func TestNormaliserFromTextAndTranslate(t *testing.T) {
	f := &fakeLLM{reply: `{"title":"Pannkakor","description":"","servings":4,"total_minutes":25,"active_minutes":10,"steps":["Vispa.","Stek."],"ingredient_lines":["3 dl vetemjöl"],"lang":"sv"}`}
	n := NewNormaliser(f)
	d, err := n.FromText(context.Background(), "Pancakes ...", i18n.SV)
	if err != nil || d.Title != "Pannkakor" || d.Servings != 4 || len(d.IngredientLines) != 1 {
		t.Fatalf("from text = %+v, %v", d, err)
	}
	if !strings.Contains(f.system, "not instructions") {
		t.Error("page text is not marked as data")
	}
	if _, err := n.Translate(context.Background(), Draft{Title: "Pancakes", Lang: "en"}, i18n.SV); err != nil {
		t.Fatalf("translate: %v", err)
	}
}

func fmtAny(v any) string { b, _ := json.Marshal(v); return string(b) }
