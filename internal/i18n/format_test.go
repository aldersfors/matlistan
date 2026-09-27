package i18n

import (
	"testing"
	"time"
)

func TestFormat(t *testing.T) {
	en, sv := mustLoad(t, EN), mustLoad(t, SV)
	d := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
	cases := []struct{ name, got, want string }{
		{"en date", en.Date(d), "Sep 28"},
		{"sv date", sv.Date(d), "28 sep"},
		{"sv weekday", sv.WeekdayShort(time.Monday), "Mån"},
		{"en sunday", en.WeekdayShort(time.Sunday), "Sun"},
		{"sv week", sv.WeekLabel(40), "Vecka 40"},
		{"en 1.5", en.Quantity(1.5), "1.5"},
		{"sv 1.5", sv.Quantity(1.5), "1,5"},
		{"whole", sv.Quantity(2), "2"},
		{"rounded", en.Quantity(0.333), "0.33"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// Keys built at runtime (weekday, month, format) must exist in every locale; a missing one
// would show as [key] on screen, for example every March.
func TestFormatKeysExistInEveryLocale(t *testing.T) {
	keys := []string{"format.date", "format.decimal_separator", "week.label"}
	for _, d := range _weekdayKeys {
		keys = append(keys, "weekday.short."+d)
	}
	for _, m := range _monthKeys {
		keys = append(keys, "month.short."+m)
	}
	for _, l := range Supported {
		c := mustLoad(t, l)
		for _, k := range keys {
			if _, ok := c.Message(k); !ok {
				t.Errorf("%s lacks %q", l, k)
			}
		}
	}
}
