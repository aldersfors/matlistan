package weekplan

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aldersfors/matlistan/internal/household"
)

func TestKeys(t *testing.T) {
	if _, err := NewKey(2026, 53); err != nil {
		t.Fatalf("2026 has week 53: %v", err)
	}
	if _, err := NewKey(2027, 53); err == nil {
		t.Fatal("2027 has no week 53")
	}
	for _, bad := range [][2]int{{2026, 0}, {1999, 10}, {2101, 1}} {
		if _, err := NewKey(bad[0], bad[1]); err == nil {
			t.Errorf("%v accepted", bad)
		}
	}
	k := Key{2026, 53}
	if got := k.AddWeeks(1); got != (Key{2027, 1}) {
		t.Errorf("W53+1 = %v", got)
	}
	if got := (Key{2027, 1}).AddWeeks(-6); got != (Key{2026, 48}) {
		t.Errorf("2027-W01-6 = %v", got)
	}
	if got := k.Monday(time.UTC).Format(time.DateOnly); got != "2026-12-28" {
		t.Errorf("monday = %s", got)
	}
	if KeyOf(time.Date(2027, 1, 3, 9, 0, 0, 0, time.UTC)) != k {
		t.Error("KeyOf Sunday 2027-01-03")
	}
	if !(Key{2026, 52}).Less(k) || k.Less(Key{2026, 52}) || k.String() != "2026-W53" {
		t.Error("Less or String")
	}
}

func TestDefaultContext(t *testing.T) {
	c := DefaultContext(5)
	for i, d := range c.Days {
		if d.Skip != (i >= 5) {
			t.Errorf("day %d skip = %v", i+1, d.Skip)
		}
	}
}

func TestContextValidate(t *testing.T) {
	c := DefaultContext(7)
	c.Days[2].Guests = 21
	c.Days[4].Away = []int64{9}
	e := c.Validate([]int64{1, 2})
	if e["days.2.guests"] != "form.out_of_range" || e["days.4.away"] != "form.unknown_option" {
		t.Fatalf("errors = %v", e)
	}
}

func TestSpecs(t *testing.T) {
	members := []household.Member{{ID: 1, BirthYear: 1985}, {ID: 2, BirthYear: 1984},
		{ID: 3, BirthYear: 2014}, {ID: 4, BirthYear: 2020}}
	st := household.Settings{DinnersPerWeek: 7, WeeknightMinutes: 45, RepeatWindowWeeks: 6}
	c := DefaultContext(7)
	c.Days[2].Busy = true       // Wednesday
	c.Days[4].Guests = 2        // Friday
	c.Days[1].Away = []int64{2} // Tuesday, one adult away
	c.Days[6].Skip = true       // Sunday
	s := Specs(Key{2026, 40}, time.UTC, c, members, st)
	// 1 + 1 + 0.75 (12 years) + 0.5 (6 years) = 3.25 -> 4
	want := []struct {
		servings, minutes int
		planned           bool
	}{{4, 45, true}, {3, 45, true}, {4, 20, true}, {4, 45, true}, {6, 45, true},
		{4, 1440, true}, {4, 1440, false}}
	for i, w := range want {
		d := s[i]
		if d.Day != i+1 || d.Servings != w.servings || d.MaxMinutes != w.minutes ||
			d.Planned != w.planned {
			t.Errorf("day %d = %+v, want %+v", i+1, d, w)
		}
	}
	if s[0].Date.Format(time.DateOnly) != "2026-09-28" {
		t.Errorf("monday = %s", s[0].Date)
	}
}

func TestParseUseUp(t *testing.T) {
	got := ParseUseUp("  halv grädde \n\n Ris,   kokt\nhalv GRÄDDE\r\n")
	if strings.Join(got, "|") != "halv grädde|Ris, kokt" {
		t.Fatalf("got %q", got)
	}
	if got := ParseUseUp(" \n "); got != nil {
		t.Fatalf("blank = %q", got)
	}
}

// Review focus: the list stays small, so the prompt stays small.
func TestUseUpLimits(t *testing.T) {
	c := DefaultContext(7)
	c.UseUp = make([]string, 11)
	for i := range c.UseUp {
		c.UseUp[i] = fmt.Sprintf("sak %d", i)
	}
	if e := c.Validate(nil); e["use_up"] == "" {
		t.Errorf("11 items accepted: %v", e)
	}
	c.UseUp = []string{strings.Repeat("a", 61)}
	if e := c.Validate(nil); e["use_up"] == "" {
		t.Errorf("61 characters accepted: %v", e)
	}
	c.UseUp = []string{strings.Repeat("ö", 60)}
	if e := c.Validate(nil); len(e) != 0 {
		t.Errorf("60 characters refused: %v", e)
	}
}
