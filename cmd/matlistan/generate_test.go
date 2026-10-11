package main

import (
	"testing"

	"github.com/aldersfors/matlistan/internal/weekplan"
)

// The scheduled run plans a week once: dinners the family set by hand ahead of it do not
// count, a planner run or an approval does.
func TestAlreadyPlanned(t *testing.T) {
	typed := []weekplan.Entry{{Day: 5, Title: "Hämtmat", Locked: true}}
	for _, c := range []struct {
		name string
		p    weekplan.Plan
		want bool
	}{
		{"hand-set dinners only", weekplan.Plan{Status: weekplan.StatusDraft, Entries: typed}, false},
		{"planned", weekplan.Plan{Status: weekplan.StatusDraft, Generated: true, Entries: typed}, true},
		{"approved", weekplan.Plan{Status: weekplan.StatusApproved, Entries: typed}, true},
	} {
		if got := alreadyPlanned(c.p); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}
