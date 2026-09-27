// Package i18n loads the embedded message catalogs and formats text for one locale.
package i18n

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
)

//go:embed locales/*.json
var _locales embed.FS

// Locale is a supported deployment language.
type Locale string

// Supported locales. EN is the default.
const (
	EN Locale = "en"
	SV Locale = "sv"
)

// Supported lists every locale with a catalog, default first.
var Supported = []Locale{EN, SV}

// ParseLocale accepts a supported locale code; "" means EN.
func ParseLocale(s string) (Locale, error) {
	if s == "" {
		return EN, nil
	}
	if i := slices.Index(Supported, Locale(s)); i >= 0 {
		return Supported[i], nil
	}
	return "", fmt.Errorf("unsupported locale %q (want one of en, sv)", s)
}

// Message is one catalog entry. A plain string sets Other only.
type Message struct {
	One   string `json:"one"`
	Other string `json:"other"`
}

// Plural reports whether the entry has a one/other pair.
func (m Message) Plural() bool { return m.One != "" }

// Catalog holds the messages of one locale.
type Catalog struct {
	locale Locale
	msgs   map[string]Message
}

// Load reads the embedded catalog for l.
func Load(l Locale) (*Catalog, error) {
	b, err := _locales.ReadFile("locales/" + string(l) + ".json")
	if err != nil {
		return nil, fmt.Errorf("catalog %s: %w", l, err)
	}
	msgs, err := parse(b)
	if err != nil {
		return nil, fmt.Errorf("catalog %s: %w", l, err)
	}
	return &Catalog{locale: l, msgs: msgs}, nil
}

func parse(b []byte) (map[string]Message, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	out := make(map[string]Message, len(raw))
	var errs []error
	for _, k := range slices.Sorted(maps.Keys(raw)) {
		var s string
		if json.Unmarshal(raw[k], &s) == nil {
			out[k] = Message{Other: s}
			continue
		}
		var m Message
		if err := json.Unmarshal(raw[k], &m); err != nil || m.One == "" || m.Other == "" {
			errs = append(errs, fmt.Errorf("%q: want a string or {\"one\", \"other\"}", k))
			continue
		}
		out[k] = m
	}
	return out, errors.Join(errs...)
}

// Locale is the catalog's language.
func (c *Catalog) Locale() Locale { return c.locale }

// Keys lists every message key, sorted.
func (c *Catalog) Keys() []string { return slices.Sorted(maps.Keys(c.msgs)) }

// Message returns the raw entry for key.
func (c *Catalog) Message(key string) (Message, bool) {
	m, ok := c.msgs[key]
	return m, ok
}

// T returns the message for key with {name} placeholders filled from args, given as
// name, value pairs. A missing key renders as [key], so it is visible in review.
func (c *Catalog) T(key string, args ...any) string {
	m, ok := c.msgs[key]
	if !ok {
		return "[" + key + "]"
	}
	return fill(m.Other, args)
}

// N is T for plural entries: n picks the form (one when n == 1) and fills {n}.
func (c *Catalog) N(key string, n int, args ...any) string {
	m, ok := c.msgs[key]
	if !ok {
		return "[" + key + "]"
	}
	s := m.Other
	if n == 1 && m.One != "" {
		s = m.One
	}
	return fill(s, append([]any{"n", n}, args...))
}

var _placeholder = regexp.MustCompile(`\{([a-z_]+)\}`)

func fill(s string, args []any) string {
	if len(args)%2 != 0 {
		panic("invariant violated: i18n args must be name, value pairs")
	}
	vals := make(map[string]string, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		name, ok := args[i].(string)
		if !ok {
			panic("invariant violated: i18n args must be name, value pairs")
		}
		vals[name] = fmt.Sprint(args[i+1])
	}
	return _placeholder.ReplaceAllStringFunc(s, func(m string) string {
		if v, ok := vals[m[1:len(m)-1]]; ok {
			return v
		}
		return m
	})
}
