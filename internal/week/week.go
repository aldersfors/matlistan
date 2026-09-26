// Package week computes the planning week.
package week

import "time"

// Week is one ISO week, Monday first.
type Week struct {
	Year, Number int
	Days         [7]time.Time
}

// Upcoming is the ISO week after the one holding now, with each day at noon in now's
// location (noon keeps the date stable across DST changes).
func Upcoming(now time.Time) Week {
	d := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, now.Location())
	sinceMonday := (int(d.Weekday()) + 6) % 7
	monday := d.AddDate(0, 0, 7-sinceMonday)
	var w Week
	for i := range w.Days {
		w.Days[i] = monday.AddDate(0, 0, i)
	}
	w.Year, w.Number = monday.ISOWeek()
	return w
}
