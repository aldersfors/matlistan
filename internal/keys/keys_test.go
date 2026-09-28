package keys

import (
	"strings"
	"testing"
)

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Pumpasoppa":                   "pumpasoppa",
		"Äppelpaj med vaniljsås":       "appelpaj-med-vaniljsas",
		"  Kött & potatis!! ":          "kott-potatis",
		"Crème brûlée":                 "creme-brulee",
		"Anna-Karin":                   "anna-karin",
		"???":                          "",
		strings.Repeat("långkok ", 20): "langkok-langkok-langkok-langkok-langkok-langkok-lang",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
		if got := Slug(in); got != "" && !Pattern.MatchString(got+"-99") {
			t.Errorf("Slug(%q)+suffix %q does not fit the key pattern", in, got)
		}
	}
}
