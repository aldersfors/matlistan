package shopping

import (
	"strings"

	"github.com/aldersfors/matlistan/internal/i18n"
)

// Line is "1,5 dl vispgrädde", "salt, efter smak" or "1 st koriander (valfri)".
func Line(c *i18n.Catalog, name string, q float64, unit string, optional bool) string {
	line := name + ", " + c.T("recipe.to_taste")
	if q > 0 {
		line = strings.TrimSpace(c.Quantity(q)+" "+c.Unit(unit)) + " " + name
	}
	if optional {
		line += " (" + c.T("recipe.optional") + ")"
	}
	return line
}

// ItemLine formats a list item; manual items are shown as typed.
func ItemLine(c *i18n.Catalog, it Item) string {
	if it.Manual {
		return it.Name
	}
	return Line(c, it.Name, it.Quantity, it.Unit, it.Optional)
}

// Text is the plain-text export: a title, then each section's unticked items.
func Text(c *i18n.Catalog, l *List) string {
	var b strings.Builder
	if l == nil {
		b.WriteString(c.T("app.name") + "\n\n" + c.T("shopping.export.none") + "\n")
		return b.String()
	}
	b.WriteString(c.T("shopping.export.title", "n", l.Key.Week) + "\n")
	section, wrote := "", false
	for _, it := range l.Items {
		if it.Checked {
			continue
		}
		if it.Section != section || !wrote {
			section, wrote = it.Section, true
			b.WriteString("\n" + c.Section(section) + "\n")
		}
		b.WriteString("- " + ItemLine(c, it) + "\n")
	}
	switch {
	case !wrote && len(l.Items) > 0:
		b.WriteString("\n" + c.T("shopping.all_bought") + "\n")
	case !wrote:
		b.WriteString("\n" + c.T("shopping.empty") + "\n")
	}
	return b.String()
}
