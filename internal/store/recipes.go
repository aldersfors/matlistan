package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/recipes"
)

const _recipeListMax = 200

// CreateRecipe stores a recipe with its ingredients and returns its id.
func (s *Store) CreateRecipe(ctx context.Context, r recipes.Recipe) (int64, error) {
	var id int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var err error
		id, err = createRecipeTx(ctx, tx, r)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("create recipe: %w", err)
	}
	return id, nil
}

func createRecipeTx(ctx context.Context, tx pgx.Tx, r recipes.Recipe) (int64, error) {
	var id int64
	if err := tx.QueryRow(ctx, `INSERT INTO recipes (title, title_key, description, lang,
		servings, active_minutes, total_minutes, tags, steps, diets, allergens, source,
		source_url)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13) RETURNING id`,
		r.Title, recipes.TitleKey(r.Title), r.Description, string(r.Lang), r.Servings,
		r.ActiveMinutes, r.TotalMinutes, orEmpty(r.Tags), orEmpty(r.Steps), orEmpty(r.Diets),
		orEmpty(r.Allergens), r.Source, nullIfEmpty(r.SourceURL)).Scan(&id); err != nil {
		return 0, err
	}
	return id, insertIngredients(ctx, tx, id, r.Ingredients)
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
	err := s.pool.QueryRow(ctx, `SELECT r.id, r.title, r.description, r.lang, r.servings,
		r.active_minutes, r.total_minutes, r.tags, r.steps, r.diets, r.allergens, r.source,
		coalesce(r.source_url, ''), coalesce(rt.average, 0), coalesce(rt.n, 0)
		FROM recipes r`+_ratingsJoin+`WHERE r.id = $1 AND r.archived_at IS NULL`, id).Scan(&r.ID,
		&r.Title, &r.Description, &lang, &r.Servings, &r.ActiveMinutes, &r.TotalMinutes, &r.Tags,
		&r.Steps, &r.Diets, &r.Allergens, &r.Source, &r.SourceURL, &r.Rating.Average,
		&r.Rating.Count)
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
	rows, err := s.pool.Query(ctx, `SELECT r.id, r.title, r.total_minutes, r.tags,
		coalesce(rt.average, 0), coalesce(rt.n, 0) FROM recipes r`+_ratingsJoin+`
		WHERE r.archived_at IS NULL AND r.lang = $1 AND`+_keptGenerated+`
		AND strpos(r.title_key, $2) > 0
		ORDER BY r.title_key, r.id LIMIT $3`, string(lang), recipes.TitleKey(query), _recipeListMax)
	if err != nil {
		return nil, fmt.Errorf("list recipes: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (recipes.Summary, error) {
		var sm recipes.Summary
		err := row.Scan(&sm.ID, &sm.Title, &sm.TotalMinutes, &sm.Tags, &sm.Rating.Average,
			&sm.Rating.Count)
		return sm, err
	})
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

// FindRecipeBySourceURL returns the live recipe imported from url.
func (s *Store) FindRecipeBySourceURL(ctx context.Context, url string) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM recipes WHERE source_url = $1 AND archived_at IS NULL`, url).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("find recipe by source url: %w", err)
	}
	return id, nil
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
