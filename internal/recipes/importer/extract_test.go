package importer

import (
	"os"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExtractGraph(t *testing.T) {
	d, ok := extractRecipe(fixture(t, "ica.html"))
	if !ok || d.Title != "Köttbullar med potatismos" || d.Description != "Klassiker & favorit" ||
		d.Servings != 4 || d.ActiveMinutes != 20 || d.TotalMinutes != 45 || d.Lang != "sv" {
		t.Fatalf("draft = %+v", d)
	}
	if len(d.IngredientLines) != 4 || d.IngredientLines[2] != "½ dl ströbröd" {
		t.Errorf("ingredients = %q", d.IngredientLines)
	}
	if len(d.Steps) != 2 || d.Steps[0] != "Blanda färsen." {
		t.Errorf("steps = %q", d.Steps)
	}
}

func TestExtractArrayAndTypeList(t *testing.T) {
	d, ok := extractRecipe(fixture(t, "arla.html"))
	if !ok || d.Title != "Pannkakor" || d.Servings != 4 || d.TotalMinutes != 25 ||
		d.ActiveMinutes != 10 || len(d.Steps) != 1 {
		t.Fatalf("draft = %+v", d)
	}
}

func TestExtractLanguageAndDurations(t *testing.T) {
	d, ok := extractRecipe(fixture(t, "koket.html"))
	if !ok || d.Lang != "en" || d.Servings != 6 || d.TotalMinutes != 65 || len(d.Steps) < 2 {
		t.Fatalf("draft = %+v", d)
	}
}

func TestExtractSections(t *testing.T) {
	d, _ := extractRecipe(fixture(t, "sections.html"))
	if strings.Join(d.Steps, "|") != "Koka såsen.|Servera." {
		t.Fatalf("steps = %q", d.Steps)
	}
}

// Review focus 4.
func TestExtractSkipsBrokenBlocks(t *testing.T) {
	if d, ok := extractRecipe(fixture(t, "broken.html")); !ok || d.Title == "" {
		t.Fatalf("draft = %+v, %v", d, ok)
	}
}

func TestNoJSONLDFallsBackToText(t *testing.T) {
	if _, ok := extractRecipe(fixture(t, "nojsonld.html")); ok {
		t.Fatal("found a recipe without JSON-LD")
	}
	text, lang := pageText(fixture(t, "nojsonld.html"))
	if lang != "sv" || strings.Contains(text, "var x") || strings.Contains(text, "Meny") ||
		strings.Contains(text, "Kontakt") || !strings.Contains(text, "vetemjöl") {
		t.Fatalf("text = %q lang %q", text, lang)
	}
	if long, _ := pageText([]byte("<main>" + strings.Repeat("ä ", 40000) + "</main>")); len([]rune(long)) > _textMax {
		t.Errorf("text not capped: %d runes", len([]rune(long)))
	}
}

func TestISOMinutes(t *testing.T) {
	for in, want := range map[string]int{"PT35M": 35, "PT1H15M": 75, "P0DT1H5M": 65, "PT2H": 120,
		"PT90S": 1, "": 0, "35 min": 0, "P1D": 1440} {
		if got := isoMinutes(in); got != want {
			t.Errorf("%q = %d, want %d", in, got, want)
		}
	}
}
