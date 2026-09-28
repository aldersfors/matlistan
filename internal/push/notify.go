package push

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/rs/zerolog"
)

var _sent = promauto.NewCounterVec(prometheus.CounterOpts{Name: "matlistan_push_sent_total",
	Help: "Web push messages by result: ok, gone (the device unsubscribed) or error."},
	[]string{"result"})

// Message is what the service worker shows. URL is a path on this site.
type Message struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

// Store is the subscriptions table.
type Store interface {
	ListPushSubscriptions(ctx context.Context) ([]Subscription, error)
	DeletePushSubscription(ctx context.Context, endpoint string) error
	MarkPushSent(ctx context.Context, endpoint string, at time.Time) error
}

// Counts is how one Notify went.
type Counts struct{ OK, Gone, Errors int }

// Notifier sends one message to every subscription, one at a time, and keeps going when
// a device fails.
type Notifier struct {
	Store  Store
	Sender Sender
	Now    func() time.Time
	Log    zerolog.Logger
}

// Notify returns an error only when the subscriptions cannot be read or the message
// cannot be encoded; per-device failures are counted and logged.
func (n Notifier) Notify(ctx context.Context, m Message) (Counts, error) {
	payload, err := json.Marshal(m)
	if err != nil {
		return Counts{}, fmt.Errorf("push: encode: %w", err)
	}
	subs, err := n.Store.ListPushSubscriptions(ctx)
	if err != nil {
		return Counts{}, fmt.Errorf("push: list subscriptions: %w", err)
	}
	var c Counts
	for _, sub := range subs {
		if err := CheckEndpoint(sub.Endpoint); err != nil {
			c.Errors++
			_sent.WithLabelValues("error").Inc()
			n.Log.Warn().Str("host", hostOf(sub.Endpoint)).Msg("push endpoint not allowed; skipped")
			continue
		}
		res, err := n.Sender.Send(ctx, sub, payload)
		switch {
		case err != nil:
			c.Errors++
			_sent.WithLabelValues("error").Inc()
			n.Log.Warn().Err(err).Msg("push failed")
		case res == Gone:
			c.Gone++
			_sent.WithLabelValues("gone").Inc()
			if err := n.Store.DeletePushSubscription(ctx, sub.Endpoint); err != nil {
				n.Log.Warn().Err(err).Str("host", hostOf(sub.Endpoint)).Msg("push: delete gone subscription")
			}
		default:
			c.OK++
			_sent.WithLabelValues("ok").Inc()
			if err := n.Store.MarkPushSent(ctx, sub.Endpoint, n.Now()); err != nil {
				n.Log.Warn().Err(err).Str("host", hostOf(sub.Endpoint)).Msg("push: mark sent")
			}
		}
	}
	return c, nil
}
