// Package keys makes the stable keys members and recipes are known by, in the app and in
// the household file in git.
package keys

import (
	"regexp"
	"strings"
)

// Pattern is what a key in the household file must look like.
var Pattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,59}$`)

// _slugMax leaves room for a "-N" or "-<id>" suffix inside Pattern's 60 characters.
const _slugMax = 52

var _fold = map[rune]rune{'å': 'a', 'ä': 'a', 'à': 'a', 'á': 'a', 'â': 'a', 'ö': 'o', 'ø': 'o',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e', 'ü': 'u', 'ú': 'u',
	'û': 'u', 'í': 'i', 'î': 'i', 'ç': 'c', 'ñ': 'n'}

// Slug makes a key from a name or title: lowercase letters and digits, words joined by one
// hyphen, Swedish and other accented letters folded. The migration's matlistan_slug does the
// same, so backfilled keys and new keys agree. It returns "" when nothing is left.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if f, ok := _fold[r]; ok {
			r = f
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
			dash = false
		} else if b.Len() > 0 && !dash {
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > _slugMax {
		out = strings.TrimRight(out[:_slugMax], "-")
	}
	return out
}
