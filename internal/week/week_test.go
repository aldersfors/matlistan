package week

import (
	"testing"
	"time"
)

func TestUpcoming(t *testing.T) {
	sthlm, err := time.LoadLocation("Europe/Stockholm")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name       string
		now        time.Time
		year, week int
		monday     string
	}{
		{"sunday plans next week", time.Date(2026, 9, 27, 7, 0, 0, 0, sthlm), 2026, 40, "2026-09-28"},
		{"monday plans the week after", time.Date(2026, 9, 28, 9, 0, 0, 0, sthlm), 2026, 41, "2026-10-05"},
		{"late sunday stays local", time.Date(2026, 9, 27, 23, 30, 0, 0, sthlm), 2026, 40, "2026-09-28"},
		{"week 53", time.Date(2026, 12, 27, 7, 0, 0, 0, sthlm), 2026, 53, "2026-12-28"},
		{"into next year", time.Date(2027, 1, 3, 7, 0, 0, 0, sthlm), 2027, 1, "2027-01-04"},
	}
	for _, c := range cases {
		w := Upcoming(c.now)
		if w.Year != c.year || w.Number != c.week || w.Days[0].Format(time.DateOnly) != c.monday {
			t.Errorf("%s: got %d-W%d monday %s", c.name, w.Year, w.Number,
				w.Days[0].Format(time.DateOnly))
		}
		if w.Days[6].Weekday() != time.Sunday {
			t.Errorf("%s: last day is %s", c.name, w.Days[6].Weekday())
		}
	}
}
