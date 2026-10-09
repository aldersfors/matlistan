package hack

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/testcontainers/testcontainers-go"
	tcexec "github.com/testcontainers/testcontainers-go/exec"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/aldersfors/matlistan/internal/keys"
	"github.com/aldersfors/matlistan/internal/store"
)

// seedDB starts Postgres with seed.sql copied in; psql runs a command and returns its exit
// code and output.
func seedDB(t *testing.T) (*postgres.PostgresContainer, func(args ...string) (int, string)) {
	t.Helper()
	ctx := t.Context()
	ctr, err := postgres.Run(ctx, "postgres:18-alpine", postgres.WithDatabase("matlistan"),
		postgres.WithUsername("matlistan"), postgres.WithPassword("matlistan"),
		postgres.BasicWaitStrategies())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = testcontainers.TerminateContainer(ctr) })
	seed, err := os.ReadFile("seed.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := ctr.CopyToContainer(ctx, seed, "/seed.sql", 0o644); err != nil {
		t.Fatal(err)
	}
	return ctr, func(args ...string) (int, string) {
		cmd := append([]string{"psql", "-U", "matlistan", "-d", "matlistan", "-v",
			"ON_ERROR_STOP=1"}, args...)
		// Multiplexed strips Docker's stream frame headers, which otherwise land in the text.
		code, out, err := ctr.Exec(ctx, cmd, tcexec.Multiplexed())
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		_, _ = b.ReadFrom(out)
		return code, b.String()
	}
}

// Review focus 5: the seed script truncates every table, so it must refuse to run unless
// the caller says it is a dev database.
func TestSeedRefusesWithoutDevFlag(t *testing.T) {
	_, psql := seedDB(t)
	if code, out := psql("-c", "CREATE TABLE members (id int); INSERT INTO members VALUES (1)"); code != 0 {
		t.Fatalf("setup failed: %s", out)
	}
	// psql's \quit has no exit status, so the proof is the message and the untouched rows.
	if _, out := psql("-f", "/seed.sql"); !strings.Contains(out, "seed_dev") {
		t.Fatalf("seed ran without -v seed_dev=1:\n%s", out)
	}
	if _, rows := psql("-tAc", "SELECT count(*) FROM members"); !strings.Contains(rows, "1") {
		t.Fatalf("members after refused seed: %q", rows)
	}
}

// The seed loads into the schema the app migrates to, with the keys the app would make, so a
// new NOT NULL column cannot break `mise run dev:seed` unnoticed.
func TestSeedLoadsIntoMigratedSchema(t *testing.T) {
	ctr, psql := seedDB(t)
	url, err := ctr.ConnectionString(t.Context(), "sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(t.Context(), store.Options{URL: url, Log: zerolog.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	st.Close()
	if code, out := psql("-q", "-v", "seed_dev=1", "-f", "/seed.sql"); code != 0 {
		t.Fatalf("seed failed:\n%s", out)
	}
	_, rows := psql("-tA", "-F", "|", "-c",
		"SELECT name, key FROM members UNION ALL SELECT title, key FROM recipes")
	lines := strings.Split(strings.TrimSpace(rows), "\n")
	if len(lines) != 8 {
		t.Fatalf("want 4 members and 4 recipes, got:\n%s", rows)
	}
	for _, l := range lines {
		name, key, _ := strings.Cut(l, "|")
		if want := keys.Slug(name); key != want {
			t.Errorf("%s has key %q, want %q", name, key, want)
		}
	}
}
