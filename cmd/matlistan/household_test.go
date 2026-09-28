package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/store"
)

type recordingHousehold struct {
	synced   bool
	released bool
	members  []household.Member
	links    []string
}

func (r *recordingHousehold) SyncHousehold(_ context.Context, m []household.Member,
	_ []recipes.Recipe, links []string) (store.SyncResult, error) {
	r.synced, r.members, r.links = true, m, links
	return store.SyncResult{Members: len(m), MissingLinks: links}, nil
}

func (r *recordingHousehold) ReleaseHousehold(context.Context) (int64, error) {
	r.released = true
	return 0, nil
}

func ident(s string) string { return s }

// Review focus 1: no household file means nothing stays locked.
func TestServeWithoutHouseholdFileReleases(t *testing.T) {
	st := &recordingHousehold{}
	missing, err := syncHousehold(t.Context(), "", i18n.SV, time.Now(), ident, st, zerolog.Nop())
	if err != nil || !st.released || st.synced || missing != nil {
		t.Fatalf("released %v synced %v missing %v err %v", st.released, st.synced, missing, err)
	}
}

func TestServeSyncsTheHouseholdFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "household.yaml")
	_ = os.WriteFile(p, []byte("members:\n  - {key: anna, name: Anna, birthYear: 1985}\nrecipeURLs: [\"https://a.example/r/\"]\n"), 0o600)
	st := &recordingHousehold{}
	missing, err := syncHousehold(t.Context(), p, i18n.SV, time.Now(), ident, st, zerolog.Nop())
	if err != nil || !st.synced || len(st.members) != 1 || len(missing) != 1 {
		t.Fatalf("synced %v members %v missing %v err %v", st.synced, st.members, missing, err)
	}
}

func TestServeRefusesAMissingOrBadFile(t *testing.T) {
	st := &recordingHousehold{}
	if _, err := syncHousehold(t.Context(), "/nonexistent/household.yaml", i18n.SV, time.Now(), ident, st, zerolog.Nop()); err == nil {
		t.Fatal("missing file accepted")
	}
	p := filepath.Join(t.TempDir(), "household.yaml")
	_ = os.WriteFile(p, []byte("members:\n  - {key: Bad, name: \"\", birthYear: 1}\n"), 0o600)
	_, err := syncHousehold(t.Context(), p, i18n.SV, time.Now(), ident, st, zerolog.Nop())
	if err == nil || !strings.Contains(err.Error(), "members[Bad]") || st.synced {
		t.Fatalf("bad file: %v, synced %v", err, st.synced)
	}
}
