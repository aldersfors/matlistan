package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

// _adminURL points at the shared test container; each test gets its own database.
var _adminURL string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	ctx := context.Background()
	ctr, err := postgres.Run(ctx, "postgres:17-alpine",
		postgres.WithDatabase("matlistan"),
		postgres.WithUsername("matlistan"),
		postgres.WithPassword("matlistan"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "start postgres:", err)
		return 1
	}
	defer func() {
		if err := testcontainers.TerminateContainer(ctr); err != nil {
			fmt.Fprintln(os.Stderr, "terminate postgres:", err)
		}
	}()
	_adminURL, err = ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		fmt.Fprintln(os.Stderr, "connection string:", err)
		return 1
	}
	return m.Run()
}

// newDatabase creates an empty database and returns its URL.
func newDatabase(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	name := "t_" + hex.EncodeToString(b)
	conn, err := pgx.Connect(t.Context(), _adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }() // test cleanup only
	if _, err := conn.Exec(t.Context(), "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(_adminURL)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	return u.String()
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), Options{URL: newDatabase(t), Log: zerolog.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return s
}

func TestOpenMigratesAndIsIdempotent(t *testing.T) {
	url := newDatabase(t)
	for range 2 {
		s, err := Open(context.Background(), Options{URL: url, Log: zerolog.Nop()})
		if err != nil {
			t.Fatal(err)
		}
		var n int
		if err := s.pool.QueryRow(context.Background(),
			`SELECT count(*) FROM auth_events`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		s.Close()
	}
}

func TestInsertAuthEvent(t *testing.T) {
	s := newTestStore(t)
	err := s.InsertAuthEvent(context.Background(), AuthEvent{At: time.Now(),
		Subject: "abc", Outcome: AuthOutcomeLogin, ClaimValues: []string{"family"}})
	if err != nil {
		t.Fatal(err)
	}
}

// Review focus 3: an unreachable database gives up without echoing the URL.
func TestOpenUnreachableDoesNotLeakURL(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := Open(ctx, Options{URL: "postgres://u:hunter2@127.0.0.1:1/x", Log: zerolog.Nop()})
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("error leaks the password: %v", err)
	}
}

func TestPruneAuthEvents(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	now := time.Now()
	for _, at := range []time.Time{now.Add(-100 * 24 * time.Hour), now.Add(-time.Hour)} {
		if err := s.InsertAuthEvent(ctx, AuthEvent{At: at, Outcome: AuthOutcomeLogin}); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.PruneAuthEvents(ctx, now.Add(-90*24*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("pruned %d, %v", n, err)
	}
}
