package hack

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// Review focus 5: the seed script truncates every table, so it must refuse to run unless
// the caller says it is a dev database.
func TestSeedRefusesWithoutDevFlag(t *testing.T) {
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
	psql := func(args ...string) (int, string) {
		cmd := append([]string{"psql", "-U", "matlistan", "-d", "matlistan", "-v",
			"ON_ERROR_STOP=1"}, args...)
		code, out, err := ctr.Exec(ctx, cmd)
		if err != nil {
			t.Fatal(err)
		}
		var b bytes.Buffer
		_, _ = b.ReadFrom(out)
		return code, b.String()
	}
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
