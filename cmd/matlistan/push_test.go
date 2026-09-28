package main

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/aldersfors/matlistan/internal/i18n"
	"github.com/aldersfors/matlistan/internal/push"
	"github.com/aldersfors/matlistan/internal/weekplan"
)

type countingStore struct{ subs []push.Subscription }

func (c *countingStore) ListPushSubscriptions(context.Context) ([]push.Subscription, error) {
	return c.subs, nil
}
func (*countingStore) DeletePushSubscription(context.Context, string) error  { return nil }
func (*countingStore) MarkPushSent(context.Context, string, time.Time) error { return nil }

type recordingSender struct{ got []string }

func (r *recordingSender) Send(_ context.Context, s push.Subscription, _ []byte) (push.Result, error) {
	r.got = append(r.got, s.Endpoint)
	return push.OK, nil
}

func TestDraftMessage(t *testing.T) {
	c, _ := i18n.Load(i18n.SV)
	m := draftMessage(c, weekplan.Key{Year: 2026, Week: 41})
	if m.Title != "Veckans förslag är klart" || m.Body != "Vecka 41 väntar på ditt godkännande." ||
		m.URL != "/week?y=2026&w=41" {
		t.Fatalf("message %+v", m)
	}
}

func TestNotifyDraftSendsOncePerDevice(t *testing.T) {
	c, _ := i18n.Load(i18n.SV)
	snd := &recordingSender{}
	n := push.Notifier{Store: &countingStore{subs: []push.Subscription{{Endpoint: "https://web.push.apple.com/a"},
		{Endpoint: "https://fcm.googleapis.com/fcm/send/b"}}}, Sender: snd, Now: time.Now, Log: zerolog.Nop()}
	notifyDraft(t.Context(), n, c, weekplan.Key{Year: 2026, Week: 41}, zerolog.Nop())
	if len(snd.got) != 2 {
		t.Fatalf("sent %v", snd.got)
	}
}
