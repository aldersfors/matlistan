package openai

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aldersfors/matlistan/internal/planner"
)

// TestLive calls a real OpenAI-compatible API; it runs only with MATLISTAN_LIVE_OPENAI=1,
// MATLISTAN_MODEL and (for api.openai.com) OPENAI_API_KEY set. MATLISTAN_OPENAI_BASE_URL
// points it at another server.
func TestLive(t *testing.T) {
	if os.Getenv("MATLISTAN_LIVE_OPENAI") != "1" {
		t.Skip("set MATLISTAN_LIVE_OPENAI=1 to call the real API")
	}
	base := os.Getenv("MATLISTAN_OPENAI_BASE_URL")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	c := New(Config{APIKey: os.Getenv("OPENAI_API_KEY"), Model: os.Getenv("MATLISTAN_MODEL"),
		BaseURL: base, Timeout: 10 * time.Minute})
	r, err := c.NewSession(planner.SystemPrompt("sv"), planner.Schema()).Send(context.Background(),
		`{"week":"2026-W40","month":"October","household":[{"age":40}],"allergens":[],"diets":[],"library_share":0,"days":[{"day":1,"weekday":"Monday","planned":true,"servings":2,"max_minutes":30}],"recent":[],"candidates":[],"task":"Plan every planned day of the week."}`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := planner.ParseProposal(r.Text); err != nil {
		t.Fatalf("live answer does not parse: %v\n%s", err, r.Text)
	}
	t.Logf("usage %+v", r.Usage)
}
