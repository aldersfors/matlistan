package planner

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jalet/matlistan/internal/i18n"
)

// _allergenWords are ingredient-name hints per allergen. Words of four letters or fewer must
// start or end a word ("ost" matches "fetaost", not "rostade"); longer ones match anywhere.
// A safety net under the declared allergens, not a complete list.
var _allergenWords = map[i18n.Locale]map[string][]string{
	i18n.SV: {
		"gluten": {
			"vete", "mjöl", "pasta", "spaghetti", "bröd", "couscous", "bulgur", "råg", "korn",
			"dinkel", "tortilla", "nudlar",
		},
		"crustaceans": {"räk", "kräft", "hummer", "krabb", "languster"},
		"eggs":        {"ägg"},
		"fish": {
			"fisk", "lax", "torsk", "tonfisk", "sill", "makrill", "ansjovis", "sej", "kolja",
			"rödspätta", "strömming",
		},
		"peanuts":  {"jordnöt"},
		"soybeans": {"soja", "tofu", "edamame", "miso"},
		"milk": {
			"mjölk", "grädde", "smör", "ost", "crème fraiche", "creme fraiche", "yoghurt", "kvarg",
			"keso", "mozzarella", "parmesan", "mascarpone", "ricotta", "halloumi",
		},
		"nuts": {
			"hasselnöt", "valnöt", "cashew", "mandel", "mandlar", "pistage", "pekannöt", "paranöt",
			"macadamia", "nötmix", "nötter",
		},
		"celery":   {"selleri"},
		"mustard":  {"senap"},
		"sesame":   {"sesam", "tahini"},
		"lupin":    {"lupin"},
		"molluscs": {"mussl", "ostron", "bläckfisk", "pilgrimsmussl", "snäck"},
	},
	i18n.EN: {
		"gluten": {
			"wheat", "flour", "pasta", "spaghetti", "bread", "couscous", "bulgur", "barley", "rye",
			"spelt", "tortilla", "noodle",
		},
		"crustaceans": {"shrimp", "prawn", "crab", "lobster", "crayfish"},
		"eggs":        {"egg"},
		"fish": {
			"fish", "salmon", "cod", "tuna", "herring", "mackerel", "anchov", "haddock", "pollock",
			"trout",
		},
		"peanuts":  {"peanut"},
		"soybeans": {"soy", "tofu", "edamame", "miso"},
		"milk": {
			"milk", "cream", "butter", "cheese", "yogurt", "yoghurt", "mozzarella", "parmesan",
			"mascarpone", "ricotta", "halloumi",
		},
		"nuts": {
			"hazelnut", "walnut", "cashew", "almond", "pistachio", "pecan", "brazil nut",
			"macadamia", "mixed nuts",
		},
		"celery":   {"celery", "celeriac"},
		"mustard":  {"mustard"},
		"sesame":   {"sesame", "tahini"},
		"lupin":    {"lupin"},
		"molluscs": {"mussel", "clam", "oyster", "squid", "octopus", "scallop"},
	},
}

var _porkWords = map[i18n.Locale][]string{
	i18n.SV: {"fläsk", "bacon", "skinka", "prosciutto", "chorizo", "salami", "korv", "pancetta"},
	i18n.EN: {"pork", "bacon", "ham", "prosciutto", "chorizo", "salami", "sausage", "pancetta"},
}

// containsAllergen reports whether an ingredient name hints at allergen.
func containsAllergen(l i18n.Locale, allergen, name string) bool {
	return matchesAny(_allergenWords[l][allergen], name)
}

func suggestsPork(l i18n.Locale, name string) bool { return matchesAny(_porkWords[l], name) }

func matchesAny(words []string, name string) bool {
	name = strings.ToLower(name)
	tokens := strings.FieldsFunc(name, func(r rune) bool { return !unicode.IsLetter(r) })
	for _, w := range words {
		if utf8.RuneCountInString(w) > 4 {
			if strings.Contains(name, w) {
				return true
			}
			continue
		}
		for _, t := range tokens {
			if strings.HasPrefix(t, w) || strings.HasSuffix(t, w) {
				return true
			}
		}
	}
	return false
}
