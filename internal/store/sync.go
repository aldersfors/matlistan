package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/aldersfors/matlistan/internal/household"
	"github.com/aldersfors/matlistan/internal/recipes"
)

// SyncResult is what one sync changed; MissingLinks are the declared links with no recipe.
type SyncResult struct {
	Members, Recipes, Links, Archived int
	MissingLinks                      []string
}

// SyncHousehold makes the database match the household declared in git, in one
// transaction: members and full recipes by key (a full recipe with a link also by that
// link), links by source_url; rows git no longer declares are archived. The login link is
// never written here.
func (s *Store) SyncHousehold(ctx context.Context, members []household.Member,
	recs []recipes.Recipe, links []string) (SyncResult, error) {
	var res SyncResult
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		keys := make([]string, 0, len(members))
		for _, m := range members {
			keys = append(keys, m.Key)
			tag, err := tx.Exec(ctx, `UPDATE members SET name = $2, birth_year = $3, diets = $4,
				allergens = $5, likes = $6, dislikes = $7, managed = 'inline', archived_at = NULL
				WHERE key = $1`, m.Key, m.Name, m.BirthYear, orEmpty(m.Diets), orEmpty(m.Allergens),
				m.Likes, m.Dislikes)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 0 {
				if _, err := tx.Exec(ctx, `INSERT INTO members (key, managed, name, birth_year,
					diets, allergens, likes, dislikes) VALUES ($1, 'inline', $2, $3, $4, $5, $6, $7)`,
					m.Key, m.Name, m.BirthYear, orEmpty(m.Diets), orEmpty(m.Allergens), m.Likes,
					m.Dislikes); err != nil {
					return err
				}
			}
			res.Members++
		}
		tag, err := tx.Exec(ctx, `UPDATE members SET archived_at = now(), oidc_subject = NULL
			WHERE managed = 'inline' AND archived_at IS NULL AND NOT (key = ANY($1))`, keys)
		if err != nil {
			return err
		}
		res.Archived += int(tag.RowsAffected())

		keys = keys[:0]
		for _, r := range recs {
			keys = append(keys, r.Key)
			if err := syncRecipe(ctx, tx, r); err != nil {
				return err
			}
			res.Recipes++
		}
		if tag, err = tx.Exec(ctx, `UPDATE recipes SET archived_at = now()
			WHERE managed = 'inline' AND archived_at IS NULL AND NOT (key = ANY($1))`, keys); err != nil {
			return err
		}
		res.Archived += int(tag.RowsAffected())

		for _, link := range links {
			var id int64
			err := tx.QueryRow(ctx, `SELECT id FROM recipes WHERE source_url = $1
				ORDER BY archived_at IS NULL DESC, archived_at DESC, id DESC LIMIT 1`, link).Scan(&id)
			if errors.Is(err, pgx.ErrNoRows) {
				res.MissingLinks = append(res.MissingLinks, link)
				continue
			}
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `UPDATE recipes SET managed = 'url', archived_at = NULL
				WHERE id = $1`, id); err != nil {
				return err
			}
			res.Links++
		}
		tag, err = tx.Exec(ctx, `UPDATE recipes SET archived_at = now()
			WHERE managed = 'url' AND archived_at IS NULL AND NOT (source_url = ANY($1))`,
			orEmpty(links))
		if err != nil {
			return err
		}
		res.Archived += int(tag.RowsAffected())
		return nil
	})
	if err != nil {
		return SyncResult{}, fmt.Errorf("sync household: %w", err)
	}
	return res, nil
}

// syncRecipe updates the recipe with r's key, or else the live recipe with r's link
// (adopting it under r's key), or creates it.
func syncRecipe(ctx context.Context, tx pgx.Tx, r recipes.Recipe) error {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM recipes WHERE key = $1`, r.Key).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) && r.SourceURL != "" {
		err = tx.QueryRow(ctx, `SELECT id FROM recipes WHERE source_url = $1
			AND archived_at IS NULL`, r.SourceURL).Scan(&id)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = createRecipeTx(ctx, tx, r)
		return err
	}
	if err != nil {
		return err
	}
	if r.SourceURL != "" {
		// Another live recipe may hold the link, for example one imported in the app after
		// this one was removed from git: the unique link would fail with no pointer.
		var taken bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM recipes WHERE source_url = $1
			AND archived_at IS NULL AND id <> $2)`, r.SourceURL, id).Scan(&taken); err != nil {
			return err
		}
		if taken {
			return fmt.Errorf("recipes[%s].sourceURL: another recipe already has this link; "+
				"archive it in the app or declare that recipe instead", r.Key)
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE recipes SET key = $2, managed = 'inline', title = $3,
		title_key = $4, description = $5, lang = $6, servings = $7, active_minutes = $8,
		total_minutes = $9, tags = $10, steps = $11, diets = $12, allergens = $13,
		source_url = $14, archived_at = NULL, updated_at = now(),
		source = CASE WHEN $14::text IS NOT NULL THEN 'imported'
			WHEN source = 'imported' THEN 'manual' ELSE source END
		WHERE id = $1`,
		id, r.Key, r.Title, recipes.TitleKey(r.Title), r.Description, string(r.Lang), r.Servings,
		r.ActiveMinutes, r.TotalMinutes, orEmpty(r.Tags), orEmpty(r.Steps), orEmpty(r.Diets),
		orEmpty(r.Allergens), nullIfEmpty(r.SourceURL)); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM recipe_ingredients WHERE recipe_id = $1`, id); err != nil {
		return err
	}
	return insertIngredients(ctx, tx, id, r.Ingredients)
}

// ReleaseHousehold hands every row back to the app, for when git no longer declares a
// household: nothing stays locked.
func (s *Store) ReleaseHousehold(ctx context.Context) (int64, error) {
	var n int64
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		for _, table := range []string{"members", "recipes"} {
			tag, err := tx.Exec(ctx, `UPDATE `+table+` SET managed = '' WHERE managed <> ''`)
			if err != nil {
				return err
			}
			n += tag.RowsAffected()
		}
		return nil
	})
	if err != nil {
		return 0, fmt.Errorf("release household: %w", err)
	}
	return n, nil
}

// CreateLinkRecipe stores a recipe imported from a declared link, owned by that link.
func (s *Store) CreateLinkRecipe(ctx context.Context, r recipes.Recipe) (int64, error) {
	r.Managed = "url"
	return s.CreateRecipe(ctx, r)
}

// AdoptLink makes the live recipe with link owned by that declared link.
func (s *Store) AdoptLink(ctx context.Context, link string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE recipes SET managed = 'url'
		WHERE source_url = $1 AND archived_at IS NULL`, link)
	if err != nil {
		return fmt.Errorf("adopt link: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
