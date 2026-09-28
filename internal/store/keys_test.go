package store

import (
	"context"
	"io/fs"
	"testing"

	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/rs/zerolog"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/keys"
)

// The migration's slug and the app's must agree, or backfilled keys would not match what
// the app makes for new rows.
func TestSQLSlugMatchesGo(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, Options{URL: newDatabaseC(t), Log: zerolog.Nop()}) // as CNPG creates it
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, in := range []string{"Pumpasoppa", "Äppelpaj med vaniljsås", "  Kött & potatis!! ",
		"Åsa", "Östen", "ÄRTSOPPA MED FLÄSK", "Crème Brûlée", "Ölbräserad Högrev",
		"Crème brûlée", "???", "Långkokt högrevsgryta med rotfrukter, äpple och örter från trädgården"} {
		var got string
		if err := s.pool.QueryRow(ctx, `SELECT matlistan_slug($1)`, in).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if want := keys.Slug(in); got != want {
			t.Errorf("matlistan_slug(%q) = %q, Go says %q", in, got, want)
		}
	}
}

func TestNewRowsGetUniqueKeys(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	a, _ := s.CreateMember(ctx, household.Member{Name: "Anna", BirthYear: 1985})
	b, _ := s.CreateMember(ctx, household.Member{Name: "Anna", BirthYear: 2015})
	c, _ := s.CreateMember(ctx, household.Member{Name: "???", BirthYear: 2015})
	ma, _ := s.GetMember(ctx, a)
	mb, _ := s.GetMember(ctx, b)
	mc, _ := s.GetMember(ctx, c)
	if ma.Key != "anna" || mb.Key != "anna-2" || mc.Key != "member" || ma.Managed != "" {
		t.Fatalf("keys %q %q %q, managed %q", ma.Key, mb.Key, mc.Key, ma.Managed)
	}
	r1, _ := s.CreateRecipe(ctx, soup())
	r2, _ := s.CreateRecipe(ctx, soup())
	g1, _ := s.GetRecipe(ctx, r1)
	g2, _ := s.GetRecipe(ctx, r2)
	if g1.Key == "" || g2.Key != g1.Key+"-2" {
		t.Fatalf("recipe keys %q %q", g1.Key, g2.Key)
	}
}

// The backfill on a database with rows already in it, as in production: every row gets a
// key, repeats get their id appended, and an empty slug falls back.
func TestMigrationBackfillsKeys(t *testing.T) {
	ctx := context.Background()
	url := newDatabaseC(t) // the production database's locale
	s, err := Open(ctx, Options{URL: url, Log: zerolog.Nop()})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cfg := s.pool.Config()
	db := stdlib.OpenDB(*cfg.ConnConfig)
	defer func() { _ = db.Close() }()
	sub, _ := fs.Sub(_migrations, "migrations")
	p, err := goose.NewProvider(goose.DialectPostgres, db, sub)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.DownTo(ctx, 8); err != nil {
		t.Fatal(err)
	}
	var ids []int64
	for _, name := range []string{"Anna", "Anna", "Ölänning", "???", "Anna 2", "Åsa"} {
		var id int64
		if err := s.pool.QueryRow(ctx, `INSERT INTO members (name, birth_year) VALUES ($1, 1990)
			RETURNING id`, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	if _, err := s.pool.Exec(ctx, `INSERT INTO recipes (title, title_key, lang, servings, active_minutes,
		total_minutes, steps, source) VALUES ('Pumpasoppa', 'pumpasoppa', 'sv', 4, 10, 30, '{x}', 'manual'),
		('Pumpasoppa', 'pumpasoppa', 'sv', 4, 10, 30, '{x}', 'manual')`); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("migrate up with rows: %v", err)
	}
	// "Anna 2" is a natural anna-2, so the second Anna must not take that key.
	want := []string{"anna", "anna-3", "olanning", "member", "anna-2", "asa"}
	for i, id := range ids {
		var key string
		if err := s.pool.QueryRow(ctx, `SELECT key FROM members WHERE id = $1`, id).Scan(&key); err != nil {
			t.Fatal(err)
		}
		if key != want[i] {
			t.Errorf("member %d key %q, want %q", id, key, want[i])
		}
	}
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(DISTINCT key) FROM recipes`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("recipe keys distinct = %d, %v", n, err)
	}
}
