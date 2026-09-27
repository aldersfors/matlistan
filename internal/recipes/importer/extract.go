package importer

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const _textMax = 30000

// Draft is a recipe as read from a page, before ingredients are normalised.
type Draft struct {
	Title, Description                    string
	Servings, TotalMinutes, ActiveMinutes int
	Steps, IngredientLines                []string
	Lang                                  string // BCP 47 from the page, "" if unknown
}

var (
	_iso    = regexp.MustCompile(`^P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$`)
	_firstN = regexp.MustCompile(`\d+`)
)

// isoMinutes reads an ISO 8601 duration; seconds round up to a minute.
func isoMinutes(s string) int {
	m := _iso.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil || s == "P" || s == "PT" || s == "" {
		return 0
	}
	n := func(i int) int { v, _ := strconv.Atoi(m[i]); return v }
	mins := n(1)*1440 + n(2)*60 + n(3)
	if n(4) > 0 {
		mins++
	}
	return mins
}

// extractRecipe finds the first schema.org Recipe with ingredients in the page's JSON-LD.
func extractRecipe(page []byte) (Draft, bool) {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return Draft{}, false
	}
	lang := htmlLang(doc)
	var found Draft
	ok := false
	walk(doc, func(n *html.Node) bool {
		if ok || !isLDScript(n) {
			return !ok
		}
		var v any
		if json.Unmarshal([]byte(textOf(n)), &v) != nil {
			return true // a broken block does not hide a good one
		}
		if r, yes := findRecipe(v); yes {
			found, ok = draftFrom(r), true
			if found.Lang == "" {
				found.Lang = lang
			}
		}
		return !ok
	})
	return found, ok && len(found.IngredientLines) > 0
}

func isLDScript(n *html.Node) bool {
	return n.Type == html.ElementNode && n.Data == "script" &&
		attr(n, "type") == "application/ld+json"
}

func findRecipe(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case []any:
		for _, e := range t {
			if r, ok := findRecipe(e); ok {
				return r, true
			}
		}
	case map[string]any:
		if isRecipe(t["@type"]) {
			return t, true
		}
		if g, ok := t["@graph"]; ok {
			return findRecipe(g)
		}
	}
	return nil, false
}

func isRecipe(t any) bool {
	switch v := t.(type) {
	case string:
		return v == "Recipe"
	case []any:
		for _, e := range v {
			if s, _ := e.(string); s == "Recipe" {
				return true
			}
		}
	}
	return false
}

func draftFrom(r map[string]any) Draft {
	d := Draft{Title: clean(str(r["name"])), Description: clean(str(r["description"])),
		Lang: str(r["inLanguage"])}
	d.Servings = yield(r["recipeYield"])
	d.ActiveMinutes = isoMinutes(str(r["prepTime"]))
	d.TotalMinutes = isoMinutes(str(r["totalTime"]))
	if d.TotalMinutes == 0 {
		d.TotalMinutes = d.ActiveMinutes + isoMinutes(str(r["cookTime"]))
	}
	for _, l := range list(r["recipeIngredient"]) {
		if s := clean(str(l)); s != "" {
			d.IngredientLines = append(d.IngredientLines, s)
		}
	}
	d.Steps = steps(r["recipeInstructions"])
	return d
}

func yield(v any) int {
	for _, e := range list(v) {
		switch t := e.(type) {
		case float64:
			return int(t)
		case string:
			if n, err := strconv.Atoi(_firstN.FindString(t)); err == nil {
				return n
			}
		}
	}
	return 0
}

func steps(v any) []string {
	var out []string
	for _, e := range list(v) {
		switch t := e.(type) {
		case string:
			out = append(out, splitSentences(clean(t))...)
		case map[string]any:
			if isType(t["@type"], "HowToSection") {
				out = append(out, steps(t["itemListElement"])...)
			} else if s := clean(str(t["text"])); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

// splitSentences keeps a single instruction string as one step unless it has line breaks.
func splitSentences(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func isType(t any, want string) bool { s, _ := t.(string); return s == want }

func list(v any) []any {
	if l, ok := v.([]any); ok {
		return l
	}
	if v == nil {
		return nil
	}
	return []any{v}
}

func str(v any) string { s, _ := v.(string); return s }

// clean decodes entities and drops tags from a JSON-LD string.
func clean(s string) string {
	if s == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader("<p>" + s + "</p>"))
	if err != nil {
		return strings.TrimSpace(s)
	}
	return strings.Join(strings.Fields(textOf(doc)), " ")
}

func htmlLang(doc *html.Node) string {
	lang := ""
	walk(doc, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "html" {
			lang = attr(n, "lang")
			return false
		}
		return true
	})
	return lang
}

var _skip = map[string]bool{"script": true, "style": true, "nav": true, "header": true,
	"footer": true, "noscript": true, "svg": true, "form": true, "aside": true}

// pageText is the page's visible main text for the model, capped at _textMax runes.
func pageText(page []byte) (string, string) {
	doc, err := html.Parse(bytes.NewReader(page))
	if err != nil {
		return "", ""
	}
	root := doc
	for _, tag := range []string{"main", "article", "body"} {
		if n := first(doc, tag); n != nil {
			root = n
			break
		}
	}
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && _skip[n.Data] {
			return
		}
		if n.Type == html.TextNode {
			if t := strings.TrimSpace(n.Data); t != "" {
				b.WriteString(t)
				b.WriteString("\n")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(root)
	text := b.String()
	if utf8.RuneCountInString(text) > _textMax {
		text = string([]rune(text)[:_textMax])
	}
	return text, htmlLang(doc)
}

func first(n *html.Node, tag string) *html.Node {
	var out *html.Node
	walk(n, func(m *html.Node) bool {
		if m.Type == html.ElementNode && m.Data == tag {
			out = m
			return false
		}
		return true
	})
	return out
}

// walk visits nodes depth-first while fn returns true.
func walk(n *html.Node, fn func(*html.Node) bool) bool {
	if !fn(n) {
		return false
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if !walk(c, fn) {
			return false
		}
	}
	return true
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textOf(n *html.Node) string {
	var b strings.Builder
	walk(n, func(m *html.Node) bool {
		if m.Type == html.TextNode {
			b.WriteString(m.Data)
		}
		return true
	})
	return b.String()
}
