package push

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
)

type memStore struct {
	mu      sync.Mutex
	subs    []Subscription
	deleted []string
	sent    []string
}

func (m *memStore) ListPushSubscriptions(context.Context) ([]Subscription, error) {
	return append([]Subscription{}, m.subs...), nil
}
func (m *memStore) DeletePushSubscription(_ context.Context, e string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deleted = append(m.deleted, e)
	return nil
}
func (m *memStore) MarkPushSent(_ context.Context, e string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, e)
	return nil
}

type scriptedSender struct {
	results map[string]Result
	errs    map[string]error
	got     map[string][]byte
}

func (s *scriptedSender) Send(_ context.Context, sub Subscription, payload []byte) (Result, error) {
	if s.got == nil {
		s.got = map[string][]byte{}
	}
	s.got[sub.Endpoint] = payload
	return s.results[sub.Endpoint], s.errs[sub.Endpoint]
}

const (
	_a = "https://web.push.apple.com/a"
	_b = "https://fcm.googleapis.com/fcm/send/b"
	_c = "https://updates.push.services.mozilla.com/wpush/v2/c"
)

func TestNotifySendsToEverySubscription(t *testing.T) {
	st := &memStore{subs: []Subscription{{Endpoint: _a}, {Endpoint: _b}}}
	snd := &scriptedSender{results: map[string]Result{_a: OK, _b: Gone}}
	n := Notifier{Store: st, Sender: snd, Now: time.Now, Log: zerolog.Nop()}
	c, err := n.Notify(t.Context(), Message{Title: "Veckans förslag är klart",
		Body: "Vecka 41 väntar på ditt godkännande.", URL: "/week?y=2026&w=41"})
	if err != nil || c != (Counts{OK: 1, Gone: 1}) {
		t.Fatalf("Notify = %+v, %v", c, err)
	}
	var m Message
	if err := json.Unmarshal(snd.got[_a], &m); err != nil || m.URL != "/week?y=2026&w=41" {
		t.Fatalf("payload %s", snd.got[_a])
	}
	if len(st.deleted) != 1 || st.deleted[0] != _b || len(st.sent) != 1 || st.sent[0] != _a {
		t.Fatalf("deleted %v sent %v", st.deleted, st.sent)
	}
}

// Review focus 3: one failing device does not stop the others.
func TestNotifyContinuesAfterAFailure(t *testing.T) {
	st := &memStore{subs: []Subscription{{Endpoint: _a}, {Endpoint: _b}, {Endpoint: _c}}}
	snd := &scriptedSender{results: map[string]Result{_c: OK}, errs: map[string]error{_a: errors.New("push to web.push.apple.com: status 500"), _b: errors.New("push to fcm.googleapis.com: timeout")}}
	c, err := Notifier{Store: st, Sender: snd, Now: time.Now, Log: zerolog.Nop()}.Notify(t.Context(), Message{})
	if err != nil || c != (Counts{OK: 1, Errors: 2}) || len(st.deleted) != 0 {
		t.Fatalf("Notify = %+v, %v, deleted %v", c, err, st.deleted)
	}
}

// Review focus 4: a stored endpoint that fails the rules is never contacted.
func TestNotifySkipsEndpointsThatFailTheRules(t *testing.T) {
	st := &memStore{subs: []Subscription{{Endpoint: "https://evil.example/p"}, {Endpoint: _a}}}
	snd := &scriptedSender{results: map[string]Result{_a: OK}}
	c, _ := Notifier{Store: st, Sender: snd, Now: time.Now, Log: zerolog.Nop()}.Notify(t.Context(), Message{})
	if _, contacted := snd.got["https://evil.example/p"]; contacted || c != (Counts{OK: 1, Errors: 1}) {
		t.Fatalf("contacted %v, counts %+v", contacted, c)
	}
}
