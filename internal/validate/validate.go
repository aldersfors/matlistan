// Package validate collects form field errors. Each value is an i18n key.
package validate

import (
	"slices"
	"strconv"
	"unicode/utf8"
)

// Errors maps a field name to the i18n key of its first problem. Empty means valid.
type Errors map[string]string

// Add records key for field unless the field already has an error.
func (e Errors) Add(field, key string) {
	if _, ok := e[field]; !ok {
		e[field] = key
	}
}

// Merge adds every error in o that e does not already have for that field.
func (e Errors) Merge(o Errors) {
	for k, v := range o {
		e.Add(k, v)
	}
}

// Text checks v's length in runes. lo 0 makes the field optional.
func (e Errors) Text(field, v string, lo, hi int) {
	switch n := utf8.RuneCountInString(v); {
	case n < lo:
		e.Add(field, "form.required")
	case n > hi:
		e.Add(field, "form.too_long")
	}
}

// Range checks lo <= v <= hi.
func (e Errors) Range(field string, v, lo, hi int) {
	if v < lo || v > hi {
		e.Add(field, "form.out_of_range")
	}
}

// Options checks that every value in vs is one of allowed.
func (e Errors) Options(field string, vs, allowed []string) {
	for _, v := range vs {
		if !slices.Contains(allowed, v) {
			e.Add(field, "form.unknown_option")
			return
		}
	}
}

// Field names an entry of a repeated group: Field("ingredients", 2, "name").
func Field(list string, i int, name string) string {
	return list + "." + strconv.Itoa(i) + "." + name
}
