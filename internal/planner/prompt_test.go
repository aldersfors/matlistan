package planner

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jalet/matlistan/internal/household"
	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/weekplan"
)

func sampleRequest() Request {
	c := weekplan.DefaultContext(7)
	c.Days[2].Busy = true
	members := []household.Member{{ID: 1, Name: "Anna", BirthYear: 1985},
		{ID: 2, Name: "Leo", BirthYear: 2014, Allergens: []string{"nuts"}}}
	k := weekplan.Key{Year: 2026, Week: 40}
	return Request{Key: k, Locale: i18n.SV, Month: time.October,
		Days:      weekplan.Specs(k, time.UTC, c, members, household.DefaultSettings()),
		Members:   []Member{{Age: 41}, {Age: 12, Allergens: []string{"nuts"}, Likes: "tacos"}},
		Allergens: []string{"nuts"}, Settings: household.DefaultSettings(),
		Recent:     []string{"Köttbullar"},
		Candidates: []Candidate{{ID: 7, Title: "Ärtsoppa", TotalMinutes: 40, WeeksSinceCooked: 9}}}
}

func TestSystemPromptPerLocale(t *testing.T) {
	if !strings.Contains(SystemPrompt(i18n.SV), "Swedish") ||
		!strings.Contains(SystemPrompt(i18n.EN), "English") {
		t.Fatal("system prompt does not name the output language")
	}
	if first, again := SystemPrompt(i18n.SV), SystemPrompt(i18n.SV); first != again {
		t.Fatal("system prompt is not stable (it must stay cacheable)")
	}
}

func TestUserPromptCarriesTheRulesAndNoNames(t *testing.T) {
	p, err := UserPrompt(sampleRequest())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"allergens":["nuts"]`, `"max_minutes":20`, `"Köttbullar"`,
		`"id":7`, `"weeks_since_cooked":9`, `"month":"October"`, `"likes":"tacos"`} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %s", want)
		}
	}
	for _, name := range []string{"Anna", "Leo"} {
		if strings.Contains(p, name) {
			t.Errorf("prompt contains member name %q", name)
		}
	}
}

// The API rejects numeric and length constraints in structured-output schemas.
func TestSchemaUsesOnlySupportedKeywords(t *testing.T) {
	raw, _ := json.Marshal(Schema())
	for _, bad := range []string{"minimum", "maximum", "minLength", "maxLength", "minItems",
		"maxItems", "multipleOf"} {
		if strings.Contains(string(raw), `"`+bad+`"`) {
			t.Errorf("schema uses %s", bad)
		}
	}
	if !strings.Contains(string(raw), `"additionalProperties":false`) {
		t.Error("objects must set additionalProperties false")
	}
}

func TestParseProposal(t *testing.T) {
	p, err := ParseProposal(`{"days":[{"day":1,"why":"x","library_recipe_id":7,"new_recipe":null}]}`)
	if err != nil || len(p.Days) != 1 || *p.Days[0].LibraryRecipeID != 7 {
		t.Fatalf("parse = %+v, %v", p, err)
	}
	if _, err := ParseProposal("Here is your plan!"); err == nil {
		t.Fatal("prose accepted")
	}
}
