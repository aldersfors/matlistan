package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/jalet/matlistan/internal/household"
)

const _memberCols = `id, name, birth_year, coalesce(oidc_subject, ''), diets, allergens, likes,
	dislikes`

func scanMember(row pgx.CollectableRow) (household.Member, error) {
	var m household.Member
	err := row.Scan(&m.ID, &m.Name, &m.BirthYear, &m.Subject, &m.Diets, &m.Allergens, &m.Likes,
		&m.Dislikes)
	return m, err
}

// ListMembers returns the active members, oldest first.
func (s *Store) ListMembers(ctx context.Context) ([]household.Member, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+_memberCols+` FROM members
		WHERE archived_at IS NULL ORDER BY birth_year, id`)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	ms, err := pgx.CollectRows(rows, scanMember)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	return ms, nil
}

// GetMember returns an active member.
func (s *Store) GetMember(ctx context.Context, id int64) (household.Member, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+_memberCols+` FROM members
		WHERE id = $1 AND archived_at IS NULL`, id)
	if err != nil {
		return household.Member{}, fmt.Errorf("get member: %w", err)
	}
	m, err := pgx.CollectExactlyOneRow(rows, scanMember)
	if errors.Is(err, pgx.ErrNoRows) {
		return household.Member{}, ErrNotFound
	}
	if err != nil {
		return household.Member{}, fmt.Errorf("get member: %w", err)
	}
	return m, nil
}

// CreateMember stores a new member and returns its id. The subject is set by LinkMember.
func (s *Store) CreateMember(ctx context.Context, m household.Member) (int64, error) {
	var id int64
	err := s.pool.QueryRow(ctx, `INSERT INTO members
		(name, birth_year, diets, allergens, likes, dislikes)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		m.Name, m.BirthYear, orEmpty(m.Diets), orEmpty(m.Allergens), m.Likes, m.Dislikes).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create member: %w", err)
	}
	return id, nil
}

// UpdateMember replaces an active member's details (not its login link).
func (s *Store) UpdateMember(ctx context.Context, m household.Member) error {
	tag, err := s.pool.Exec(ctx, `UPDATE members SET name = $2, birth_year = $3, diets = $4,
		allergens = $5, likes = $6, dislikes = $7 WHERE id = $1 AND archived_at IS NULL`,
		m.ID, m.Name, m.BirthYear, orEmpty(m.Diets), orEmpty(m.Allergens), m.Likes, m.Dislikes)
	if err != nil {
		return fmt.Errorf("update member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ArchiveMember hides a member and releases its login link. History keeps the row.
func (s *Store) ArchiveMember(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `UPDATE members SET archived_at = now(), oidc_subject = NULL
		WHERE id = $1 AND archived_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("archive member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// LinkMember makes subject belong to member id, moving it from any other member.
func (s *Store) LinkMember(ctx context.Context, id int64, subject string) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE members SET oidc_subject = NULL
			WHERE oidc_subject = $1`, subject); err != nil {
			return fmt.Errorf("link member: %w", err)
		}
		tag, err := tx.Exec(ctx, `UPDATE members SET oidc_subject = $2
			WHERE id = $1 AND archived_at IS NULL`, id, subject)
		if err != nil {
			return fmt.Errorf("link member: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// GetSettings returns the planning rules.
func (s *Store) GetSettings(ctx context.Context) (household.Settings, error) {
	var st household.Settings
	err := s.pool.QueryRow(ctx, `SELECT dinners_per_week, weeknight_minutes,
		repeat_window_weeks, library_share FROM household_settings WHERE id = 1`).
		Scan(&st.DinnersPerWeek, &st.WeeknightMinutes, &st.RepeatWindowWeeks, &st.LibraryShare)
	if err != nil {
		return household.Settings{}, fmt.Errorf("get settings: %w", err)
	}
	return st, nil
}

// UpdateSettings replaces the planning rules.
func (s *Store) UpdateSettings(ctx context.Context, st household.Settings) error {
	_, err := s.pool.Exec(ctx, `UPDATE household_settings SET dinners_per_week = $1,
		weeknight_minutes = $2, repeat_window_weeks = $3, library_share = $4 WHERE id = 1`,
		st.DinnersPerWeek, st.WeeknightMinutes, st.RepeatWindowWeeks, st.LibraryShare)
	if err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	return nil
}

// ListStaples returns the staples in alphabetical order.
func (s *Store) ListStaples(ctx context.Context) ([]household.Staple, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, name FROM staples ORDER BY name_key`)
	if err != nil {
		return nil, fmt.Errorf("list staples: %w", err)
	}
	st, err := pgx.CollectRows(rows, pgx.RowToStructByPos[household.Staple])
	if err != nil {
		return nil, fmt.Errorf("list staples: %w", err)
	}
	return st, nil
}

// AddStaple stores name unless an equal staple (StapleKey) exists.
func (s *Store) AddStaple(ctx context.Context, name string) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO staples (name, name_key) VALUES ($1, $2)
		ON CONFLICT (name_key) DO NOTHING`, household.StapleName(name), household.StapleKey(name))
	if err != nil {
		return fmt.Errorf("add staple: %w", err)
	}
	return nil
}

// RemoveStaple deletes a staple.
func (s *Store) RemoveStaple(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM staples WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("remove staple: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
