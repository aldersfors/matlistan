package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/theme"
)

func inputCSS(t *testing.T) (tokens, rest string) {
	t.Helper()
	b, err := os.ReadFile("../../web/styles/input.css")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	start, end := strings.Index(s, "/* tokens:start */"), strings.Index(s, "/* tokens:end */")
	if start < 0 || end < start {
		t.Fatal("tokens markers missing")
	}
	return s[start:end], s[:start] + s[end:]
}

func TestInputCSSColorsAreTokens(t *testing.T) {
	_, rest := inputCSS(t)
	if m := regexp.MustCompile(`#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(`).FindString(rest); m != "" {
		t.Errorf("colour %q outside the tokens block", m)
	}
}

func TestEveryThemeTokenHasADefault(t *testing.T) {
	tokens, _ := inputCSS(t)
	for _, tok := range theme.Tokens {
		if !strings.Contains(tokens, theme.Var(tok)+":") {
			t.Errorf("%s has no default in input.css", theme.Var(tok))
		}
	}
}

// Review focus 1 (template side): every literal key a view uses exists in the catalog.
func TestViewsUseKnownKeys(t *testing.T) {
	en, err := i18n.Load(i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	// Any quoted dotted literal whose first segment is a catalog prefix counts, so keys passed
	// to components as arguments are checked too.
	re := regexp.MustCompile(`"([a-z_]+(?:\.[a-z0-9_]+)+\.?)"`)
	prefixes := map[string]bool{}
	for _, k := range en.Keys() {
		prefixes[strings.SplitN(k, ".", 2)[0]] = true
	}
	files, _ := filepath.Glob("views/*.templ")
	if len(files) == 0 {
		t.Fatal("no templ files found")
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			if strings.HasSuffix(m[1], ".") || !prefixes[strings.SplitN(m[1], ".", 2)[0]] {
				continue
			}
			if _, ok := en.Message(m[1]); !ok {
				t.Errorf("%s uses unknown key %q", f, m[1])
			}
		}
	}
}

func TestVendoredFilesMatchChecksums(t *testing.T) {
	b, err := os.ReadFile("static/SHA256SUMS")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		data, err := os.ReadFile(filepath.Join("static", f[1]))
		if err != nil {
			t.Errorf("%s listed but missing", f[1])
			continue
		}
		if sum := sha256.Sum256(data); hex.EncodeToString(sum[:]) != f[0] {
			t.Errorf("%s does not match its SHA256SUMS entry", f[1])
		}
	}
}

// Keys passed as literals from Go code (handlers, the auth package, views' helper funcs) must
// exist too; TestViewsUseKnownKeys only reads .templ files.
func TestGoCodeUsesKnownKeys(t *testing.T) {
	en, err := i18n.Load(i18n.EN)
	if err != nil {
		t.Fatal(err)
	}
	re := regexp.MustCompile(`\b(?:i18n\.)?[TN]\((?:ctx, |r\.Context\(\), )?"([a-z0-9_.]+)"`)
	var files []string
	err = filepath.WalkDir("..", func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") &&
			!strings.HasSuffix(path, "_templ.go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil || len(files) == 0 {
		t.Fatalf("walk: %v (%d files)", err, len(files))
	}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			if strings.HasSuffix(m[1], ".") {
				continue // a prefix joined at runtime; TestFormatKeysExistInEveryLocale covers it
			}
			if _, ok := en.Message(m[1]); !ok {
				t.Errorf("%s uses unknown key %q", f, m[1])
			}
		}
	}
}

func TestCookScriptIsSafe(t *testing.T) {
	b, err := os.ReadFile("static/cook.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"eval(", "innerHTML", "new Function"} {
		if strings.Contains(string(b), bad) {
			t.Errorf("cook.js uses %s", bad)
		}
	}
}
