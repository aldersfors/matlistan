package importer

import (
	"context"
	"os"
	"testing"

	"github.com/aldersfors/matlistan/internal/i18n"
)

// TestLive fetches a real recipe page; it runs only with MATLISTAN_LIVE_IMPORT=1 and a URL
// in MATLISTAN_IMPORT_URL. It uses no model, so it needs no key.
func TestLive(t *testing.T) {
	u := os.Getenv("MATLISTAN_IMPORT_URL")
	if os.Getenv("MATLISTAN_LIVE_IMPORT") != "1" || u == "" {
		t.Skip("set MATLISTAN_LIVE_IMPORT=1 and MATLISTAN_IMPORT_URL to fetch a real page")
	}
	r, notes, err := Importer{Fetch: NewFetcher("Matlistan/live-test"), Lang: i18n.SV}.Import(context.Background(), u)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%q: %d ingredients, %d steps, notes %v", r.Title, len(r.Ingredients), len(r.Steps), notes)
	if r.Title == "" || len(r.Ingredients) == 0 {
		t.Fatal("empty recipe")
	}
}
