// Package weekplan models one planned week: its key, the household's conditions per day and
// the dinners chosen.
package weekplan

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/recipes"
	"github.com/aldersfors/matlistan/internal/validate"
)

// Key is an ISO week.
type Key struct{ Year, Week int }

var errKey = errors.New("no such ISO week")

// NewKey accepts a week that exists in the year (1-52, or 53 in long years), 2000-2100.
func NewKey(year, week int) (Key, error) {
	if year < 2000 || year > 2100 || week < 1 {
		return Key{}, errKey
	}
	k := Key{year, week}
	if KeyOf(k.Monday(time.UTC)) != k {
		return Key{}, errKey
	}
	return k, nil
}

// KeyOf is the ISO week that holds t.
func KeyOf(t time.Time) Key {
	y, w := t.ISOWeek()
	return Key{y, w}
}

// Monday is noon on the week's Monday in loc.
func (k Key) Monday(loc *time.Location) time.Time {
	jan4 := time.Date(k.Year, time.January, 4, 12, 0, 0, 0, loc)
	week1 := jan4.AddDate(0, 0, -((int(jan4.Weekday()) + 6) % 7))
	return week1.AddDate(0, 0, 7*(k.Week-1))
}

// AddWeeks moves n weeks forward (negative: back).
func (k Key) AddWeeks(n int) Key { return KeyOf(k.Monday(time.UTC).AddDate(0, 0, 7*n)) }

// Less orders keys in time.
func (k Key) Less(o Key) bool { return k.Year < o.Year || (k.Year == o.Year && k.Week < o.Week) }

func (k Key) String() string { return fmt.Sprintf("%d-W%02d", k.Year, k.Week) }

// Status is where a plan is in its life.
type Status string

// Plan statuses.
const (
	StatusDraft    Status = "draft"
	StatusApproved Status = "approved"
)

// DayContext is what the household says about one day.
type DayContext struct {
	Skip   bool    `json:"skip"`
	Busy   bool    `json:"busy"`
	Guests int     `json:"guests"`
	Away   []int64 `json:"away"`
}

// Context holds the seven days, Monday first.
type Context struct {
	Days [7]DayContext `json:"days"`
	// UseUp is what is already at home and should be cooked first, as the household wrote it.
	UseUp []string `json:"use_up,omitempty"`
}

const (
	guestsMax  = 20
	useUpMax   = 10
	useUpChars = 60
)

// ParseUseUp reads one item per line, tidies spaces and drops blank and repeated lines.
func ParseUseUp(text string) []string {
	var out []string
	seen := map[string]bool{}
	for line := range strings.Lines(text) {
		item := strings.Join(strings.Fields(line), " ")
		if key := strings.ToLower(item); item != "" && !seen[key] {
			seen[key] = true
			out = append(out, item)
		}
	}
	return out
}

// DefaultContext plans dinnersPerWeek days and marks the rest of the week as no dinner.
func DefaultContext(dinnersPerWeek int) Context {
	var c Context
	for i := range c.Days {
		c.Days[i].Skip = i >= dinnersPerWeek
		c.Days[i].Away = []int64{}
	}
	return c
}

// Validate checks guests and that everyone away is a member.
func (c Context) Validate(memberIDs []int64) validate.Errors {
	e := validate.Errors{}
	for i, d := range c.Days {
		e.Range(validate.Field("days", i, "guests"), d.Guests, 0, guestsMax)
		for _, id := range d.Away {
			if !slices.Contains(memberIDs, id) {
				e.Add(validate.Field("days", i, "away"), "form.unknown_option")
			}
		}
	}
	if len(c.UseUp) > useUpMax {
		e.Add("use_up", "week.use_up_too_many")
	}
	for _, item := range c.UseUp {
		e.Text("use_up", item, 1, useUpChars)
	}
	return e
}

// Entry is one planned dinner as shown.
type Entry struct {
	Day                    int
	RecipeID               int64
	Title                  string
	TotalMinutes, Servings int
	Why                    string
	Locked                 bool // kept when the rest of the draft is planned again
}

// Plan is a week with its conditions and dinners.
type Plan struct {
	ID         int64
	Key        Key
	Status     Status
	Context    Context
	Error      string
	Entries    []Entry
	ApprovedBy string
}

// Entry returns the dinner planned for day (1..7).
func (p Plan) Entry(day int) (Entry, bool) {
	for _, e := range p.Entries {
		if e.Day == day {
			return e, true
		}
	}
	return Entry{}, false
}

// Pick is the planner's decision for one day.
type Pick struct {
	Day      int
	RecipeID int64
	New      *recipes.Recipe
	Servings int
	Why      string
}

// Cooked is a dinner from an approved week.
type Cooked struct {
	Key      Key
	RecipeID int64
	Title    string
}

// DaySpec is one day as the planner sees it.
type DaySpec struct {
	Day                          int
	Date                         time.Time
	Planned, Busy                bool
	Guests, Servings, MaxMinutes int
}

// BusyMinutes is the time limit of a quick evening.
const BusyMinutes = 20

const noLimit = 1440

// Specs turns the week's conditions into per-day servings and time limits.
func Specs(k Key, loc *time.Location, c Context, members []household.Member,
	s household.Settings) [7]DaySpec {
	var out [7]DaySpec
	monday := k.Monday(loc)
	for i, d := range c.Days {
		date := monday.AddDate(0, 0, i)
		portions := float64(d.Guests)
		for _, m := range members {
			if !slices.Contains(d.Away, m.ID) {
				portions += household.PortionFactor(household.Age(m.BirthYear, date))
			}
		}
		limit := s.WeeknightMinutes
		if i >= 5 {
			limit = noLimit
		}
		if d.Busy {
			limit = min(limit, BusyMinutes)
		}
		out[i] = DaySpec{Day: i + 1, Date: date, Planned: !d.Skip, Busy: d.Busy,
			Guests: d.Guests, MaxMinutes: limit,
			Servings: min(20, max(1, int(math.Ceil(portions-1e-9))))}
	}
	return out
}
