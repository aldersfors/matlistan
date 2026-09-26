package i18n

import (
	"math"
	"strconv"
	"strings"
	"time"
)

var (
	_weekdayKeys = [7]string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
	_monthKeys   = [12]string{"jan", "feb", "mar", "apr", "may", "jun",
		"jul", "aug", "sep", "oct", "nov", "dec"}
)

// WeekdayShort is the short weekday name: "Mon", "Mån".
func (c *Catalog) WeekdayShort(d time.Weekday) string {
	return c.T("weekday.short." + _weekdayKeys[(int(d)+6)%7])
}

// Date renders the day and short month in the locale's order: "Sep 28", "28 sep".
func (c *Catalog) Date(t time.Time) string {
	return c.T("format.date", "day", t.Day(), "month", c.T("month.short."+_monthKeys[t.Month()-1]))
}

// WeekLabel is "Week 40" / "Vecka 40".
func (c *Catalog) WeekLabel(week int) string { return c.T("week.label", "n", week) }

// Quantity renders q with at most two decimals and the locale's decimal separator.
func (c *Catalog) Quantity(q float64) string {
	s := strconv.FormatFloat(math.Round(q*100)/100, 'f', -1, 64)
	return strings.Replace(s, ".", c.T("format.decimal_separator"), 1)
}

// Allergen is the label of an allergen key: "nuts" is "Nuts" / "Nötter".
func (c *Catalog) Allergen(key string) string { return c.T("allergen." + key) }

// Diet is the label of a diet key.
func (c *Catalog) Diet(key string) string { return c.T("diet." + key) }
