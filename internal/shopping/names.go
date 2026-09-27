package shopping

import (
	"strings"

	"github.com/aldersfors/matlistan/internal/household"
)

// _singular maps plural (and a few irregular) Swedish ingredient names to the singular the
// list shows, so the same food from two recipes is one line. It is a list, not grammar
// rules: rules would also turn "mjölk" into "mjöl".
var _singular = map[string]string{
	"morötter": "morot", "tomater": "tomat", "körsbärstomater": "körsbärstomat",
	"lökar": "lök", "gula lökar": "gul lök", "rödlökar": "rödlök", "schalottenlökar": "schalottenlök",
	"purjolökar": "purjolök", "vårlökar": "vårlök", "potatisar": "potatis", "paprikor": "paprika",
	"citroner": "citron", "limefrukter": "lime", "apelsiner": "apelsin", "äpplen": "äpple",
	"bananer": "banan", "gurkor": "gurka", "zucchinis": "zucchini",
	"auberginer": "aubergine", "champinjoner": "champinjon", "avokador": "avokado",
	"avokados": "avokado", "vitlöksklyftor": "vitlöksklyfta", "chilifrukter": "chili",
	"palsternackor": "palsternacka", "rödbetor": "rödbeta", "sötpotatisar": "sötpotatis",
	"kycklingfiléer": "kycklingfilé", "tortillas": "tortillabröd",
}

// baseName is the name an ingredient is listed and compared under: its singular when known,
// otherwise the name as written, with spaces tidied.
func baseName(name string) string {
	clean := household.StapleName(name)
	if s, ok := _singular[strings.ToLower(clean)]; ok {
		return s
	}
	return clean
}

// nameKey compares ingredients and staples.
func nameKey(name string) string { return household.StapleKey(baseName(name)) }
