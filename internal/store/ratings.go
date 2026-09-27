package store

import (
	"context"
	"fmt"

	"github.com/aldersfors/matlistan/internal/weekplan"
)

// _ratingsJoin adds each recipe's rating as columns average and n (0 when unrated).
const _ratingsJoin = ` LEFT JOIN (SELECT recipe_id, avg(score)::float8 AS average,
	count(*)::int AS n FROM ratings GROUP BY recipe_id) rt ON rt.recipe_id = r.id `

// _keptGenerated keeps generated recipes out of the library until they are well rated.
const _keptGenerated = ` (r.source <> 'generated' OR coalesce(rt.average, 0) >= 4) `

// SetRating stores a member's score for an approved week's dinner, replacing an earlier one.
func (s *Store) SetRating(ctx context.Context, k weekplan.Key, day int, memberID int64,
	score int) error {
	tag, err := s.pool.Exec(ctx, `INSERT INTO ratings (plan_id, day, member_id, recipe_id, score)
		SELECT p.id, e.day, m.id, e.recipe_id, $5
		FROM week_plans p JOIN plan_entries e ON e.plan_id = p.id AND e.day = $3
		JOIN members m ON m.id = $4 AND m.archived_at IS NULL
		WHERE p.iso_year = $1 AND p.iso_week = $2 AND p.status = 'approved'
		ON CONFLICT (plan_id, day, member_id) DO UPDATE SET score = EXCLUDED.score,
		rated_at = now()`, k.Year, k.Week, day, memberID, score)
	if err != nil {
		return fmt.Errorf("set rating: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// WeekRatings returns day -> member -> score for week k.
func (s *Store) WeekRatings(ctx context.Context, k weekplan.Key) (map[int]map[int64]int, error) {
	rows, err := s.pool.Query(ctx, `SELECT rt.day, rt.member_id, rt.score FROM ratings rt
		JOIN week_plans p ON p.id = rt.plan_id WHERE p.iso_year = $1 AND p.iso_week = $2`,
		k.Year, k.Week)
	if err != nil {
		return nil, fmt.Errorf("week ratings: %w", err)
	}
	defer rows.Close()
	out := map[int]map[int64]int{}
	for rows.Next() {
		var day, score int
		var member int64
		if err := rows.Scan(&day, &member, &score); err != nil {
			return nil, fmt.Errorf("week ratings: %w", err)
		}
		if out[day] == nil {
			out[day] = map[int64]int{}
		}
		out[day][member] = score
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("week ratings: %w", err)
	}
	return out, nil
}
