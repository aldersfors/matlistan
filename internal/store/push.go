package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/aldersfors/matlistan/internal/push"
)

const _pushSubscriptionsMax = 20

// SavePushSubscription stores a device's subscription, or updates it when the endpoint is
// already known. A new endpoint past the limit is ErrTooMany. Errors never include the
// endpoint.
func (s *Store) SavePushSubscription(ctx context.Context, subject string,
	sub push.Subscription) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// Serialise concurrent saves so two new devices cannot both pass the count.
		_, err := tx.Exec(ctx, `LOCK TABLE push_subscriptions IN SHARE ROW EXCLUSIVE MODE`)
		if err != nil {
			return err
		}
		var exists bool
		var n int
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM push_subscriptions WHERE endpoint = $1),
			(SELECT count(*) FROM push_subscriptions)`, sub.Endpoint).Scan(&exists, &n); err != nil {
			return err
		}
		if !exists && n >= _pushSubscriptionsMax {
			return ErrTooMany
		}
		_, err = tx.Exec(ctx, `INSERT INTO push_subscriptions (owner_subject, endpoint, p256dh, auth)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (endpoint) DO UPDATE SET owner_subject = EXCLUDED.owner_subject,
				p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth`,
			subject, sub.Endpoint, sub.P256DH, sub.Auth)
		return err
	})
	if errors.Is(err, ErrTooMany) {
		return ErrTooMany
	}
	if err != nil {
		return errors.New("save push subscription failed") // no endpoint in the message
	}
	return nil
}

// DeletePushSubscription removes one device.
func (s *Store) DeletePushSubscription(ctx context.Context, endpoint string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM push_subscriptions WHERE endpoint = $1`, endpoint)
	if err != nil {
		return errors.New("delete push subscription failed")
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ListPushSubscriptions returns every device.
func (s *Store) ListPushSubscriptions(ctx context.Context) ([]push.Subscription, error) {
	rows, err := s.pool.Query(ctx, `SELECT endpoint, p256dh, auth FROM push_subscriptions
		ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions: %w", err)
	}
	out, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (push.Subscription, error) {
		var sub push.Subscription
		err := r.Scan(&sub.Endpoint, &sub.P256DH, &sub.Auth)
		return sub, err
	})
	if err != nil {
		return nil, fmt.Errorf("list push subscriptions: %w", err)
	}
	return out, nil
}

// CountPushSubscriptions is how many devices have notifications on.
func (s *Store) CountPushSubscriptions(ctx context.Context) (int, error) {
	var n int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM push_subscriptions`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count push subscriptions: %w", err)
	}
	return n, nil
}

// MarkPushSent records a delivered message.
func (s *Store) MarkPushSent(ctx context.Context, endpoint string, at time.Time) error {
	if _, err := s.pool.Exec(ctx, `UPDATE push_subscriptions SET last_sent_at = $2
		WHERE endpoint = $1`,
		endpoint, at); err != nil {
		return errors.New("mark push sent failed")
	}
	return nil
}
