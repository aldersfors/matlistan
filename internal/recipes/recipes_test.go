package recipes

import (
	"slices"
	"strings"
	"testing"

	"github.com/aldersfors/matlistan/internal/i18n"
)

func valid() Recipe {
	return Recipe{Title: "Köttbullar", Lang: i18n.SV, Servings: 4, ActiveMinutes: 30,
		TotalMinutes: 40, Source: "manual", Steps: []string{"Blanda", "Stek"},
		Ingredients: []Ingredient{
			{Name: "blandfärs", Quantity: 500, Unit: "g", Section: "meat_fish"},
			{Name: "salt", Section: "pantry"},
		}}
}

func TestValidRecipe(t *testing.T) {
	if e := valid().Validate(); len(e) != 0 {
		t.Fatalf("errors: %v", e)
	}
}

func TestRecipeValidate(t *testing.T) {
	cases := map[string]struct {
		edit  func(*Recipe)
		field string
		key   string
	}{
		"no title":        {func(r *Recipe) { r.Title = "" }, "title", "form.required"},
		"active > total":  {func(r *Recipe) { r.ActiveMinutes = 50 }, "active_minutes", "form.out_of_range"},
		"no servings":     {func(r *Recipe) { r.Servings = 0 }, "servings", "form.out_of_range"},
		"no ingredients":  {func(r *Recipe) { r.Ingredients = nil }, "ingredients", "form.required"},
		"no steps":        {func(r *Recipe) { r.Steps = nil }, "steps", "form.required"},
		"unit needed":     {func(r *Recipe) { r.Ingredients[0].Unit = "" }, "ingredients.0.unit", "form.unit_required"},
		"bad unit":        {func(r *Recipe) { r.Ingredients[0].Unit = "cup" }, "ingredients.0.unit", "form.unknown_option"},
		"bad section":     {func(r *Recipe) { r.Ingredients[1].Section = "aisle9" }, "ingredients.1.section", "form.unknown_option"},
		"negative amount": {func(r *Recipe) { r.Ingredients[0].Quantity = -1 }, "ingredients.0.quantity", "form.out_of_range"},
		"bad allergen":    {func(r *Recipe) { r.Allergens = []string{"cats"} }, "allergens", "form.unknown_option"},
		"bad lang":        {func(r *Recipe) { r.Lang = "de" }, "lang", "form.unknown_option"},
		"too many tags":   {func(r *Recipe) { r.Tags = make([]string, TagsMax+1) }, "tags", "form.too_long"},
		"long step":       {func(r *Recipe) { r.Steps[1] = strings.Repeat("x", 1001) }, "steps", "form.too_long"},
		"too many ingr.":  {func(r *Recipe) { r.Ingredients = slices.Repeat(r.Ingredients[:1], IngredientsMax+1) }, "ingredients", "form.too_long"},
	}
	for name, c := range cases {
		r := valid()
		r.Ingredients = slices.Clone(r.Ingredients)
		r.Steps = slices.Clone(r.Steps)
		c.edit(&r)
		if got := r.Validate()[c.field]; got != c.key {
			t.Errorf("%s: %s = %q, want %q", name, c.field, got, c.key)
		}
	}
}

func TestScaled(t *testing.T) {
	got := valid().Scaled(6)
	if got[0].Quantity != 750 || got[1].Quantity != 0 {
		t.Fatalf("scaled = %+v", got)
	}
	if valid().Ingredients[0].Quantity != 500 {
		t.Fatal("Scaled changed the recipe")
	}
}

func TestParseQuantity(t *testing.T) {
	for _, c := range []struct {
		in   string
		want float64
	}{{"1,5", 1.5}, {"1.5", 1.5}, {" 2 ", 2}, {"", 0}} {
		if got, err := ParseQuantity(c.in); err != nil || got != c.want {
			t.Errorf("%q = %v, %v", c.in, got, err)
		}
	}
	for _, in := range []string{"abc", "-1", "NaN", "Inf", "1e400"} {
		if _, err := ParseQuantity(in); err == nil {
			t.Errorf("%q accepted", in)
		}
	}
}

func TestSplit(t *testing.T) {
	if got := SplitLines("Blanda\r\n\n  Stek  \n"); !slices.Equal(got, []string{"Blanda", "Stek"}) {
		t.Errorf("lines = %q", got)
	}
	if got := SplitTags(" vardag, ,Fisk ,vardag"); !slices.Equal(got, []string{"fisk", "vardag"}) {
		t.Errorf("tags = %q", got)
	}
	if TitleKey("  Ärtsoppa ") != "ärtsoppa" {
		t.Error("title key")
	}
}

func TestCatalogsNameEveryUnitAndSection(t *testing.T) {
	for _, l := range i18n.Supported {
		c, err := i18n.Load(l)
		if err != nil {
			t.Fatal(err)
		}
		for _, u := range Units {
			if _, ok := c.Message("unit." + u); !ok {
				t.Errorf("%s lacks unit.%s", l, u)
			}
		}
		for _, s := range Sections {
			if _, ok := c.Message("section." + s); !ok {
				t.Errorf("%s lacks section.%s", l, s)
			}
		}
	}
}

func TestScores(t *testing.T) {
	if len(Scores) != 3 || Scores[0] != ScoreLoved || Scores[2] != ScoreNotAgain {
		t.Fatalf("scores = %v", Scores)
	}
}

func TestNormalizeSourceURL(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"https://www.ICA.se/recept/kottbullar-123/", "https://www.ica.se/recept/kottbullar-123"},
		{"HTTPS://www.ica.se/recept/kottbullar-123#steg", "https://www.ica.se/recept/kottbullar-123"},
		{" https://www.arla.se/recept/pannkakor/?utm=x ", "https://www.arla.se/recept/pannkakor?utm=x"},
		{"https://www.koket.se/", "https://www.koket.se"},
	} {
		if got, err := NormalizeSourceURL(c.in); err != nil || got != c.want {
			t.Errorf("%q = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "ica.se/recept", "ftp://x/y", "javascript:alert(1)", "https://",
		"https://" + strings.Repeat("a", 2000) + ".se"} {
		if _, err := NormalizeSourceURL(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSourceURLRules(t *testing.T) {
	r := valid()
	r.Source, r.SourceURL = "imported", "https://www.ica.se/recept/x"
	if e := r.Validate(); len(e) != 0 {
		t.Fatalf("imported recipe: %v", e)
	}
	r.Source = "manual"
	if r.Validate()["source"] != "form.unknown_option" {
		t.Error("manual recipe with a source URL accepted")
	}
	r.Source, r.SourceURL = "imported", ""
	if r.Validate()["source"] != "form.unknown_option" {
		t.Error("imported recipe without a source URL accepted")
	}
	r.SourceURL = "http://www.ica.se/recept/x"
	if r.Validate()["source_url"] != "form.invalid_url" {
		t.Error("http source URL accepted")
	}
}
