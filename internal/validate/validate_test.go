package validate

import "testing"

func TestErrors(t *testing.T) {
	e := Errors{}
	e.Text("name", "", 1, 60)
	e.Text("name", "x", 1, 60) // a later check never overwrites the first error
	e.Text("likes", "åäö", 0, 2)
	e.Text("bio", "", 0, 10)
	e.Range("year", 1800, 1900, 2026)
	e.Range("ok", 5, 1, 7)
	e.Options("diets", []string{"vegan", "carnivore"}, []string{"vegan"})
	e.Options("fine", []string{"vegan"}, []string{"vegan"})
	want := Errors{"name": "form.required", "likes": "form.too_long",
		"year": "form.out_of_range", "diets": "form.unknown_option"}
	if len(e) != len(want) {
		t.Fatalf("errors = %v, want %v", e, want)
	}
	for k, v := range want {
		if e[k] != v {
			t.Errorf("%s = %q, want %q", k, e[k], v)
		}
	}
}

func TestMergeKeepsFirst(t *testing.T) {
	e := Errors{"a": "form.required"}
	e.Merge(Errors{"a": "form.too_long", "b": "form.required"})
	if e["a"] != "form.required" || e["b"] != "form.required" {
		t.Fatalf("merge = %v", e)
	}
}

func TestField(t *testing.T) {
	if got := Field("ingredients", 2, "name"); got != "ingredients.2.name" {
		t.Fatalf("Field = %q", got)
	}
}
