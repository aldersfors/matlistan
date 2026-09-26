package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jalet/matlistan/internal/i18n"
	"github.com/jalet/matlistan/internal/recipes"
	"github.com/jalet/matlistan/internal/weekplan"
)

const _candidatesMax = 200

// GetPlan returns the week with its dinners in day order.
func (s *Store) GetPlan(ctx context.Context, k weekplan.Key) (weekplan.Plan, error) {
	p := weekplan.Plan{Key: k}
	var raw []byte
	var status string
	err := s.pool.QueryRow(ctx, `SELECT id, status, context, error, approved_by
		FROM week_plans WHERE iso_year = $1 AND iso_week = $2`, k.Year, k.Week).
		Scan(&p.ID, &status, &raw, &p.Error, &p.ApprovedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return weekplan.Plan{}, ErrNotFound
	}
	if err != nil {
		return weekplan.Plan{}, fmt.Errorf("get plan: %w", err)
	}
	p.Status = weekplan.Status(status)
	if err := json.Unmarshal(raw, &p.Context); err != nil {
		return weekplan.Plan{}, fmt.Errorf("get plan context: %w", err)
	}
	rows, err := s.pool.Query(ctx, `SELECT e.day, e.recipe_id, r.title, r.total_minutes,
		e.servings, e.why FROM plan_entries e JOIN recipes r ON r.id = e.recipe_id
		WHERE e.plan_id = $1 ORDER BY e.day`, p.ID)
	if err != nil {
		return weekplan.Plan{}, fmt.Errorf("get plan entries: %w", err)
	}
	p.Entries, err = pgx.CollectRows(rows, pgx.RowToStructByPos[weekplan.Entry])
	if err != nil {
		return weekplan.Plan{}, fmt.Errorf("get plan entries: %w", err)
	}
	return p, nil
}

// upsertDraft creates the week or updates its context, refusing approved weeks.
func upsertDraft(ctx context.Context, tx pgx.Tx, k weekplan.Key, c weekplan.Context) (int64,
	error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO week_plans (iso_year, iso_week, context)
		VALUES ($1, $2, $3)
		ON CONFLICT (iso_year, iso_week) DO UPDATE SET context = EXCLUDED.context
		WHERE week_plans.status = 'draft'
		RETURNING id`, k.Year, k.Week, raw).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrApproved
	}
	return id, err
}

// SaveContext stores the week's conditions.
func (s *Store) SaveContext(ctx context.Context, k weekplan.Key, c weekplan.Context) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		_, err := upsertDraft(ctx, tx, k, c)
		return err
	})
	return wrapPlanErr("save context", err)
}

// SavePicks stores the planner's dinners, creating generated recipes first. replaceAll
// clears the other days (a whole-week plan); otherwise only the picked days change (a swap).
func (s *Store) SavePicks(ctx context.Context, k weekplan.Key, c weekplan.Context,
	picks []weekplan.Pick, replaceAll bool) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		id, err := upsertDraft(ctx, tx, k, c)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE week_plans SET error = '', generated_at = now()
			WHERE id = $1`, id); err != nil {
			return err
		}
		if replaceAll {
			if _, err := tx.Exec(ctx, `DELETE FROM plan_entries WHERE plan_id = $1`, id); err != nil {
				return err
			}
		}
		for _, p := range picks {
			recipeID := p.RecipeID
			if p.New != nil {
				if recipeID, err = createRecipeTx(ctx, tx, *p.New); err != nil {
					return err
				}
			}
			if _, err := tx.Exec(ctx, `INSERT INTO plan_entries
				(plan_id, day, recipe_id, servings, why) VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (plan_id, day) DO UPDATE SET recipe_id = EXCLUDED.recipe_id,
				servings = EXCLUDED.servings, why = EXCLUDED.why`,
				id, p.Day, recipeID, p.Servings, p.Why); err != nil {
				return err
			}
		}
		return nil
	})
	return wrapPlanErr("save picks", err)
}

// SetPlanError records why the last generation failed; the dinners stay as they were.
func (s *Store) SetPlanError(ctx context.Context, k weekplan.Key, c weekplan.Context,
	key string) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		id, err := upsertDraft(ctx, tx, k, c)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE week_plans SET error = $2 WHERE id = $1`, id, key)
		return err
	})
	return wrapPlanErr("set plan error", err)
}

// ApprovePlan fixes a draft that has at least one dinner.
func (s *Store) ApprovePlan(ctx context.Context, k weekplan.Key, subject string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE week_plans SET status = 'approved',
		approved_at = now(), approved_by = $3
		WHERE iso_year = $1 AND iso_week = $2 AND status = 'draft'
		AND EXISTS (SELECT 1 FROM plan_entries WHERE plan_id = week_plans.id)`,
		k.Year, k.Week, subject)
	if err != nil {
		return fmt.Errorf("approve plan: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CookedSince lists the dinners of approved weeks with from <= week < until.
func (s *Store) CookedSince(ctx context.Context, from, until weekplan.Key) (
	[]weekplan.Cooked, error) {
	rows, err := s.pool.Query(ctx, `SELECT p.iso_year, p.iso_week, e.recipe_id, r.title
		FROM week_plans p JOIN plan_entries e ON e.plan_id = p.id
		JOIN recipes r ON r.id = e.recipe_id
		WHERE p.status = 'approved' AND (p.iso_year, p.iso_week) >= ($1, $2)
		AND (p.iso_year, p.iso_week) < ($3, $4)
		ORDER BY p.iso_year, p.iso_week, e.day`, from.Year, from.Week, until.Year, until.Week)
	if err != nil {
		return nil, fmt.Errorf("cooked since: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (weekplan.Cooked, error) {
		var c weekplan.Cooked
		err := row.Scan(&c.Key.Year, &c.Key.Week, &c.RecipeID, &c.Title)
		return c, err
	})
	if err != nil {
		return nil, fmt.Errorf("cooked since: %w", err)
	}
	return out, nil
}

// ListCandidates returns active recipes in lang for the planner, without ingredients.
func (s *Store) ListCandidates(ctx context.Context, lang i18n.Locale) ([]recipes.Recipe, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, title, total_minutes, tags, diets, allergens
		FROM recipes WHERE archived_at IS NULL AND lang = $1 ORDER BY title_key, id LIMIT $2`,
		string(lang), _candidatesMax)
	if err != nil {
		return nil, fmt.Errorf("list candidates: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (recipes.Recipe, error) {
		r := recipes.Recipe{Lang: lang}
		err := row.Scan(&r.ID, &r.Title, &r.TotalMinutes, &r.Tags, &r.Diets, &r.Allergens)
		return r, err
	})
	if err != nil {
		return nil, fmt.Errorf("list candidates: %w", err)
	}
	return out, nil
}

func wrapPlanErr(op string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrApproved):
		return ErrApproved
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}
