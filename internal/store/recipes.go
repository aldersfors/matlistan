package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/recipes"
)

const _recipeListMax = 200

// CreateRecipe stores a recipe with its ingredients and returns its id.
func (s *Store) CreateRecipe(ctx context.Context, r recipes.Recipe) (int64, error) {
	var id int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO recipes (title, title_key, description, lang,
			servings, active_minutes, total_minutes, tags, steps, diets, allergens, source)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
			r.Title, recipes.TitleKey(r.Title), r.Description, string(r.Lang), r.Servings,
			r.ActiveMinutes, r.TotalMinutes, orEmpty(r.Tags), orEmpty(r.Steps), orEmpty(r.Diets),
			orEmpty(r.Allergens), r.Source).Scan(&id); err != nil {
			return err
		}
		return insertIngredients(ctx, tx, id, r.Ingredients)
	})
	if err != nil {
		return 0, fmt.Errorf("create recipe: %w", err)
	}
	return id, nil
}

// UpdateRecipe replaces an active recipe and its ingredients.
func (s *Store) UpdateRecipe(ctx context.Context, r recipes.Recipe) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE recipes SET title = $2, title_key = $3,
			description = $4, servings = $5, active_minutes = $6, total_minutes = $7, tags = $8,
			steps = $9, diets = $10, allergens = $11, updated_at = now()
			WHERE id = $1 AND archived_at IS NULL`,
			r.ID, r.Title, recipes.TitleKey(r.Title), r.Description, r.Servings, r.ActiveMinutes,
			r.TotalMinutes, orEmpty(r.Tags), orEmpty(r.Steps), orEmpty(r.Diets),
			orEmpty(r.Allergens))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM recipe_ingredients WHERE recipe_id = $1`,
			r.ID); err != nil {
			return err
		}
		return insertIngredients(ctx, tx, r.ID, r.Ingredients)
	})
	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("update recipe: %w", err)
	}
	return nil
}

func insertIngredients(ctx context.Context, tx pgx.Tx, id int64, ins []recipes.Ingredient) error {
	rows := make([][]any, len(ins))
	for i, in := range ins {
		rows[i] = []any{id, i, in.Name, in.Quantity, in.Unit, in.Section, in.Optional}
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"recipe_ingredients"},
		[]string{"recipe_id", "position", "name", "quantity", "unit", "section", "optional"},
		pgx.CopyFromRows(rows))
	return err
}

// GetRecipe returns an active recipe with its ingredients in order.
func (s *Store) GetRecipe(ctx context.Context, id int64) (recipes.Recipe, error) {
	var r recipes.Recipe
	var lang string
	err := s.pool.QueryRow(ctx, `SELECT id, title, description, lang, servings,
		active_minutes, total_minutes, tags, steps, diets, allergens, source
		FROM recipes WHERE id = $1 AND archived_at IS NULL`, id).Scan(&r.ID, &r.Title,
		&r.Description, &lang, &r.Servings, &r.ActiveMinutes, &r.TotalMinutes, &r.Tags, &r.Steps,
		&r.Diets, &r.Allergens, &r.Source)
	if errors.Is(err, pgx.ErrNoRows) {
		return recipes.Recipe{}, ErrNotFound
	}
	if err != nil {
		return recipes.Recipe{}, fmt.Errorf("get recipe: %w", err)
	}
	r.Lang = i18n.Locale(lang)
	rows, err := s.pool.Query(ctx, `SELECT name, quantity, unit, section, optional
		FROM recipe_ingredients WHERE recipe_id = $1 ORDER BY position`, id)
	if err != nil {
		return recipes.Recipe{}, fmt.Errorf("get recipe ingredients: %w", err)
	}
	r.Ingredients, err = pgx.CollectRows(rows, pgx.RowToStructByPos[recipes.Ingredient])
	if err != nil {
		return recipes.Recipe{}, fmt.Errorf("get recipe ingredients: %w", err)
	}
	return r, nil
}

// ListRecipes returns active recipes in lang whose title contains query (any case).
func (s *Store) ListRecipes(ctx context.Context, lang i18n.Locale, query string) (
	[]recipes.Summary, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, title, total_minutes, tags FROM recipes
		WHERE archived_at IS NULL AND lang = $1 AND strpos(title_key, $2) > 0
		ORDER BY title_key, id LIMIT $3`, string(lang), recipes.TitleKey(query), _recipeListMax)
	if err != nil {
		return nil, fmt.Errorf("list recipes: %w", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[recipes.Summary])
	if err != nil {
		return nil, fmt.Errorf("list recipes: %w", err)
	}
	return out, nil
}

// ArchiveRecipe hides a recipe from the library. Past plans keep referring to it.
func (s *Store) ArchiveRecipe(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE recipes SET archived_at = now()
		WHERE id = $1 AND archived_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("archive recipe: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
