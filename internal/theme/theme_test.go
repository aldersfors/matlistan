package theme

import (
	"strings"
	"testing"
)

func TestParseRendersBothModes(t *testing.T) {
	th, err := Parse([]byte(`
light: {page: "#ffffff", day1Ink: "#0d1b2a"}
dark: {day7: "#7a5f10"}
`))
	if err != nil {
		t.Fatal(err)
	}
	want := `@media not all and (prefers-color-scheme: dark) {
  :root {
    --ml-page: #ffffff;
    --ml-day1-ink: #0d1b2a;
  }
}
@media (prefers-color-scheme: dark) {
  :root {
    --ml-day7: #7a5f10;
  }
}
`
	if got := string(th.CSS()); got != want {
		t.Fatalf("CSS =\n%s\nwant\n%s", got, want)
	}
}

func TestParseReportsEveryProblem(t *testing.T) {
	_, err := Parse([]byte(`
light: {dya1: "#fff", day2: fff}
brandBar: ["#111"]
`))
	if err == nil {
		t.Fatal("want error")
	}
	for _, want := range []string{"dya1", "day2", "brandBar"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not name %s: %v", want, err)
		}
	}
}

func TestZeroThemeIsEmpty(t *testing.T) {
	if got := (Theme{}).CSS(); len(got) != 0 {
		t.Fatalf("CSS = %q", got)
	}
}

func TestVar(t *testing.T) {
	if got := Var("day1Ink"); got != "--ml-day1-ink" {
		t.Fatalf("Var = %q", got)
	}
}
