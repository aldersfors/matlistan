package planner

import (
	"maps"
	"slices"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/recipes"
)

func object(props map[string]any) map[string]any {
	req := slices.Sorted(maps.Keys(props)) // sorted so request bodies are reproducible
	return map[string]any{"type": "object", "properties": props, "required": req,
		"additionalProperties": false}
}

func enumOf(vs []string) map[string]any { return map[string]any{"type": "string", "enum": vs} }

func arrayOf(item map[string]any) map[string]any {
	return map[string]any{"type": "array", "items": item}
}

var (
	_str = map[string]any{"type": "string"}
	_int = map[string]any{"type": "integer"}
)

// Schema is the JSON schema of a Proposal, using only keywords structured outputs support.
func Schema() map[string]any {
	units := append([]string{""}, recipes.Units...)
	ingredient := object(map[string]any{"name": _str, "quantity": map[string]any{"type": "number"},
		"unit": enumOf(units), "section": enumOf(recipes.Sections),
		"optional": map[string]any{"type": "boolean"}})
	recipe := object(map[string]any{"title": _str, "description": _str, "servings": _int,
		"active_minutes": _int, "total_minutes": _int, "tags": arrayOf(_str),
		"steps": arrayOf(_str), "diets": arrayOf(enumOf(household.Diets)),
		"allergens": arrayOf(enumOf(household.Allergens)), "ingredients": arrayOf(ingredient)})
	day := object(map[string]any{
		"day": map[string]any{"type": "integer", "enum": []int{1, 2, 3, 4, 5, 6, 7}},
		"why": _str,
		"library_recipe_id": map[string]any{"anyOf": []any{_int,
			map[string]any{"type": "null"}}},
		"new_recipe": map[string]any{"anyOf": []any{recipe, map[string]any{"type": "null"}}},
	})
	return object(map[string]any{"days": arrayOf(day)})
}
