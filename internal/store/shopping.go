package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jalet/matlistan/internal/apitoken"
	"github.com/jalet/matlistan/internal/shopping"
	"github.com/jalet/matlistan/internal/weekplan"
)

const _manualItemsMax = 100

// PlanIngredients lists every ingredient of the week's dinners with the day's servings. It
// reads archived recipes too: a planned dinner stays on the list.
func (s *Store) PlanIngredients(ctx context.Context, k weekplan.Key) ([]shopping.Use, error) {
	rows, err := s.pool.Query(ctx, `SELECT e.day, e.servings, r.servings, i.name, i.quantity,
		i.unit, i.section, i.optional
		FROM week_plans p JOIN plan_entries e ON e.plan_id = p.id
		JOIN recipes r ON r.id = e.recipe_id
		JOIN recipe_ingredients i ON i.recipe_id = r.id
		WHERE p.iso_year = $1 AND p.iso_week = $2 ORDER BY e.day, i.position`, k.Year, k.Week)
	if err != nil {
		return nil, fmt.Errorf("plan ingredients: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (shopping.Use, error) {
		var u shopping.Use
		in := &u.Ingredient
		err := row.Scan(&u.Day, &u.Servings, &u.RecipeServings, &in.Name, &in.Quantity,
			&in.Unit, &in.Section, &in.Optional)
		return u, err
	})
	if err != nil {
		return nil, fmt.Errorf("plan ingredients: %w", err)
	}
	return out, nil
}

func insertItems(ctx context.Context, tx pgx.Tx, listID int64, from int,
	items []shopping.Item) error {
	rows := make([][]any, len(items))
	for i, it := range items {
		days := it.Days
		if days == nil {
			days = []int{}
		}
		rows[i] = []any{listID, from + i, it.Name, it.Section, it.Quantity, it.Unit, days,
			it.Optional, it.Manual}
	}
	_, err := tx.CopyFrom(ctx, pgx.Identifier{"shopping_items"},
		[]string{"list_id", "position", "name", "section", "quantity", "unit", "days",
			"optional", "manual"}, pgx.CopyFromRows(rows))
	return err
}

const _itemCols = `id, name, section, quantity, unit, days, optional, manual, checked`

func scanItem(row pgx.CollectableRow) (shopping.Item, error) {
	var it shopping.Item
	err := row.Scan(&it.ID, &it.Name, &it.Section, &it.Quantity, &it.Unit, &it.Days,
		&it.Optional, &it.Manual, &it.Checked)
	return it, err
}

func (s *Store) listWhere(ctx context.Context, where string, args ...any) (shopping.List,
	error) {
	var l shopping.List
	err := s.pool.QueryRow(ctx, `SELECT l.id, p.iso_year, p.iso_week, l.excluded_staples
		FROM shopping_lists l JOIN week_plans p ON p.id = l.plan_id WHERE `+where+`
		ORDER BY p.iso_year DESC, p.iso_week DESC LIMIT 1`, args...).
		Scan(&l.ID, &l.Key.Year, &l.Key.Week, &l.Excluded)
	if errors.Is(err, pgx.ErrNoRows) {
		return shopping.List{}, ErrNotFound
	}
	if err != nil {
		return shopping.List{}, fmt.Errorf("get shopping list: %w", err)
	}
	rows, err := s.pool.Query(ctx, `SELECT `+_itemCols+` FROM shopping_items
		WHERE list_id = $1 ORDER BY position`, l.ID)
	if err != nil {
		return shopping.List{}, fmt.Errorf("get shopping items: %w", err)
	}
	if l.Items, err = pgx.CollectRows(rows, scanItem); err != nil {
		return shopping.List{}, fmt.Errorf("get shopping items: %w", err)
	}
	return l, nil
}

// GetShoppingList returns week k's list.
func (s *Store) GetShoppingList(ctx context.Context, k weekplan.Key) (shopping.List, error) {
	return s.listWhere(ctx, `p.iso_year = $1 AND p.iso_week = $2`, k.Year, k.Week)
}

// CurrentShoppingList returns the latest list of a week up to upTo.
func (s *Store) CurrentShoppingList(ctx context.Context, upTo weekplan.Key) (shopping.List,
	error) {
	return s.listWhere(ctx, `(p.iso_year, p.iso_week) <= ($1, $2)`, upTo.Year, upTo.Week)
}

// ToggleItem ticks or unticks an item and returns it.
func (s *Store) ToggleItem(ctx context.Context, id int64) (shopping.Item, error) {
	rows, err := s.pool.Query(ctx, `UPDATE shopping_items SET checked = NOT checked
		WHERE id = $1 RETURNING `+_itemCols, id)
	if err != nil {
		return shopping.Item{}, fmt.Errorf("toggle item: %w", err)
	}
	it, err := pgx.CollectExactlyOneRow(rows, scanItem)
	if errors.Is(err, pgx.ErrNoRows) {
		return shopping.Item{}, ErrNotFound
	}
	if err != nil {
		return shopping.Item{}, fmt.Errorf("toggle item: %w", err)
	}
	return it, nil
}

// AddManualItem appends something the household typed in, under "other".
func (s *Store) AddManualItem(ctx context.Context, listID int64, name string) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		var manual, next int
		err := tx.QueryRow(ctx, `SELECT count(*) FILTER (WHERE manual),
			coalesce(max(position) + 1, 0) FROM shopping_items WHERE list_id = $1`, listID).
			Scan(&manual, &next)
		if err != nil {
			return err
		}
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM shopping_lists WHERE id = $1)`,
			listID).Scan(&exists); err != nil {
			return err
		}
		switch {
		case !exists:
			return ErrNotFound
		case manual >= _manualItemsMax:
			return ErrTooMany
		}
		return insertItems(ctx, tx, listID, next, []shopping.Item{{Name: name, Section: "other",
			Manual: true}})
	})
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrTooMany) {
		return err
	}
	if err != nil {
		return fmt.Errorf("add manual item: %w", err)
	}
	return nil
}

// RemoveManualItem deletes an item the household added; planned items stay.
func (s *Store) RemoveManualItem(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM shopping_items WHERE id = $1 AND manual`, id)
	if err != nil {
		return fmt.Errorf("remove item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateAPIToken stores a key's hash.
func (s *Store) CreateAPIToken(ctx context.Context, subject, name string, hash []byte) error {
	if _, err := s.pool.Exec(ctx, `INSERT INTO api_tokens (owner_subject, name, hash)
		VALUES ($1, $2, $3)`, subject, name, hash); err != nil {
		return fmt.Errorf("create api token: %w", err)
	}
	return nil
}

// ListAPITokens returns every key, newest first, without secrets.
func (s *Store) ListAPITokens(ctx context.Context) ([]apitoken.Token, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name, created_at, last_used_at FROM api_tokens
		ORDER BY created_at DESC, id DESC`)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	out, err := pgx.CollectRows(rows, pgx.RowToStructByPos[apitoken.Token])
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	return out, nil
}

// RevokeAPIToken deletes a key.
func (s *Store) RevokeAPIToken(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM api_tokens WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("revoke api token: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// UseAPIToken reports whether hash belongs to a key, recording its use.
func (s *Store) UseAPIToken(ctx context.Context, hash []byte) (bool, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE api_tokens SET last_used_at = now() WHERE hash = $1`,
		hash)
	if err != nil {
		return false, fmt.Errorf("use api token: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}
