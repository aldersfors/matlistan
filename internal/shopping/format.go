package shopping

import (
	"encoding/json"
	"html"
	"strings"

	"github.com/aldersfors/matlistan/internal/i18n"
)

// Line is "1,5 dl vispgrädde", "salt, efter smak" or "1 st koriander (valfri)".
func Line(c *i18n.Catalog, name string, q float64, unit string, optional bool) string {
	line := name + ", " + c.T("recipe.to_taste")
	if q > 0 {
		line = amountText(c, q, unit) + " " + name
	}
	if optional {
		line += " (" + c.T("recipe.optional") + ")"
	}
	return line
}

func amountText(c *i18n.Catalog, q float64, unit string) string {
	return strings.TrimSpace(c.Quantity(q) + " " + c.Unit(unit))
}

// ItemLine formats a list item; manual items are shown as typed.
func ItemLine(c *i18n.Catalog, it Item) string {
	if it.Manual {
		return it.Name
	}
	return Line(c, it.Name, it.Quantity, it.Unit, it.Optional)
}

// export is what both exports show: a title, then each section's unticked items, or a
// sentence when there are none.
type export struct {
	title    string
	sections []exportSection
	note     string
}

type exportSection struct {
	name  string
	items []Item
}

func exportOf(c *i18n.Catalog, l *List) export {
	if l == nil {
		return export{title: c.T("app.name"), note: c.T("shopping.export.none")}
	}
	e := export{title: c.T("shopping.export.title", "n", l.Key.Week)}
	for _, it := range l.Items {
		if it.Checked {
			continue
		}
		if n := len(e.sections); n == 0 || e.sections[n-1].name != c.Section(it.Section) {
			e.sections = append(e.sections, exportSection{name: c.Section(it.Section)})
		}
		s := &e.sections[len(e.sections)-1]
		s.items = append(s.items, it)
	}
	switch {
	case len(e.sections) == 0 && len(l.Items) > 0:
		e.note = c.T("shopping.all_bought")
	case len(e.sections) == 0:
		e.note = c.T("shopping.empty")
	}
	return e
}

// Text is the plain-text export.
func Text(c *i18n.Catalog, l *List) string {
	e := exportOf(c, l)
	var b strings.Builder
	b.WriteString(e.title + "\n")
	for _, s := range e.sections {
		b.WriteString("\n" + s.name + "\n")
		for _, it := range s.items {
			b.WriteString("- " + ItemLine(c, it) + "\n")
		}
	}
	if e.note != "" {
		b.WriteString("\n" + e.note + "\n")
	}
	return b.String()
}

// HTML is the rich-text export for Apple Notes: headings per section, bullets, the amount
// in bold and "(valfri)" in italics. Every name is escaped; manual items are as typed.
func HTML(c *i18n.Catalog, l *List) string {
	e := exportOf(c, l)
	esc := html.EscapeString
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="` + string(c.Locale()) + `">` +
		`<head><meta charset="utf-8"><title>` + esc(e.title) + "</title></head>" +
		"<body><h1>" + esc(e.title) + "</h1>")
	for _, s := range e.sections {
		b.WriteString("<h2>" + esc(s.name) + "</h2><ul>")
		for _, it := range s.items {
			b.WriteString("<li>" + itemHTML(c, it) + "</li>")
		}
		b.WriteString("</ul>")
	}
	if e.note != "" {
		b.WriteString("<p>" + esc(e.note) + "</p>")
	}
	b.WriteString("</body></html>")
	return b.String()
}

func itemHTML(c *i18n.Catalog, it Item) string {
	esc := html.EscapeString
	if it.Manual {
		return esc(it.Name)
	}
	line := esc(it.Name + ", " + c.T("recipe.to_taste"))
	if it.Quantity > 0 {
		line = "<b>" + esc(amountText(c, it.Quantity, it.Unit)) + "</b> " + esc(it.Name)
	}
	if it.Optional {
		line += " <i>(" + esc(c.T("recipe.optional")) + ")</i>"
	}
	return line
}

type jsonExport struct {
	Title    string        `json:"title"`
	Sections []jsonSection `json:"sections"`
	Note     string        `json:"note,omitempty"`
}

type jsonSection struct {
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

// JSON is the export for the checklist Shortcut, which appends each section's items as Notes
// tickboxes, one per line. A line break inside an item would split it in two, so it becomes
// a space.
func JSON(c *i18n.Catalog, l *List) string {
	e := exportOf(c, l)
	out := jsonExport{Title: e.title, Sections: []jsonSection{}, Note: e.note}
	for _, s := range e.sections {
		js := jsonSection{Name: s.name}
		for _, it := range s.items {
			js.Items = append(js.Items, strings.Join(strings.Fields(ItemLine(c, it)), " "))
		}
		out.Sections = append(out.Sections, js)
	}
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false) // read by Shortcuts, never rendered as a page
	if err := enc.Encode(out); err != nil {
		panic(err) // only strings: cannot fail
	}
	return strings.TrimSuffix(b.String(), "\n")
}
