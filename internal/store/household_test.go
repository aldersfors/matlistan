package store

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/aldersfors/matlistan/internal/household"
)

func TestMembers(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	leo := household.Member{Name: "Leo", BirthYear: 2014, Allergens: []string{"nuts"},
		Diets: []string{}, Likes: "tacos"}
	id, err := s.CreateMember(ctx, leo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateMember(ctx, household.Member{Name: "Anna", BirthYear: 1985}); err != nil {
		t.Fatal(err) // nil slices must not violate NOT NULL
	}
	got, err := s.GetMember(ctx, id)
	if err != nil || got.Name != "Leo" || !slices.Equal(got.Allergens, []string{"nuts"}) ||
		got.Likes != "tacos" {
		t.Fatalf("get = %+v, %v", got, err)
	}
	got.Likes = "pizza"
	if err := s.UpdateMember(ctx, got); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListMembers(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "Anna" || list[1].Likes != "pizza" {
		t.Fatalf("list (oldest first) = %+v, %v", list, err)
	}
	if err := s.ArchiveMember(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetMember(ctx, id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived get: %v", err)
	}
	if err := s.UpdateMember(ctx, got); !errors.Is(err, ErrNotFound) {
		t.Fatalf("archived update: %v", err)
	}
	if err := s.ArchiveMember(ctx, 9999); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown archive: %v", err)
	}
}

// Review focus 4: one login links to one member; archiving the member clears the link.
func TestLinkMember(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	a, _ := s.CreateMember(ctx, household.Member{Name: "Anna", BirthYear: 1985})
	b, _ := s.CreateMember(ctx, household.Member{Name: "Erik", BirthYear: 1984})
	if err := s.LinkMember(ctx, a, "sub-1"); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkMember(ctx, b, "sub-1"); err != nil {
		t.Fatal(err)
	}
	ma, _ := s.GetMember(ctx, a)
	mb, _ := s.GetMember(ctx, b)
	if ma.Subject != "" || mb.Subject != "sub-1" {
		t.Fatalf("link moved? a=%q b=%q", ma.Subject, mb.Subject)
	}
	if err := s.ArchiveMember(ctx, b); err != nil {
		t.Fatal(err)
	}
	if err := s.LinkMember(ctx, a, "sub-1"); err != nil {
		t.Fatalf("relink after archive: %v", err)
	}
	if err := s.LinkMember(ctx, b, "sub-1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("link archived member: %v", err)
	}
}

func TestSettings(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	got, err := s.GetSettings(ctx)
	if err != nil || got != household.DefaultSettings() {
		t.Fatalf("defaults = %+v, %v", got, err)
	}
	want := household.Settings{DinnersPerWeek: 5, WeeknightMinutes: 30, RepeatWindowWeeks: 8,
		LibraryShare: 70}
	if err := s.UpdateSettings(ctx, want); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetSettings(ctx); got != want {
		t.Fatalf("settings = %+v", got)
	}
}

// Review focus 3: "salt" after "Salt" is the same staple.
func TestStaples(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	for _, n := range []string{"Salt", "olja", " salt "} {
		if err := s.AddStaple(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	list, err := s.ListStaples(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "olja" || list[1].Name != "Salt" {
		t.Fatalf("staples = %+v, %v", list, err)
	}
	if err := s.RemoveStaple(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RemoveStaple(ctx, list[0].ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second remove: %v", err)
	}
}
