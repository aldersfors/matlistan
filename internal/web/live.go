package web

import (
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/aldersfors/matlistan/internal/weekplan"
)

// _liveHeartbeat is how often an idle event stream sends a comment, so proxies keep it open.
const _liveHeartbeat = 25 * time.Second

// broker tells open week pages that their week changed. It lives in memory: the app runs as
// one replica (planning jobs are in-process too).
type broker struct {
	mu   sync.Mutex
	subs map[weekplan.Key]map[chan struct{}]struct{}
}

func newBroker() *broker { return &broker{subs: map[weekplan.Key]map[chan struct{}]struct{}{}} }

// subscribe returns a channel that receives after each change to k until cancel is called.
// Changes that arrive before the last one was received collapse into one.
func (b *broker) subscribe(k weekplan.Key) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs[k] == nil {
		b.subs[k] = map[chan struct{}]struct{}{}
	}
	b.subs[k][ch] = struct{}{}
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		delete(b.subs[k], ch)
		if len(b.subs[k]) == 0 {
			delete(b.subs, k)
		}
	}
}

// publish wakes every subscriber of k without waiting for any of them.
func (b *broker) publish(k weekplan.Key) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[k] {
		select {
		case ch <- struct{}{}:
		default: // one is already pending
		}
	}
}

// weekEvents streams "week" events to an open week page whenever the week changes.
func (s *server) weekEvents(w http.ResponseWriter, r *http.Request) {
	k, ok := s.weekKey(r)
	if !ok {
		s.badRequest(w, r)
		return
	}
	changed, cancel := s.live.subscribe(k)
	defer cancel()
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no")
	rc := http.NewResponseController(w)
	// The server's WriteTimeout would end the stream, so each write gets its own deadline.
	send := func(msg string) bool {
		err := rc.SetWriteDeadline(time.Now().Add(s.heartbeat + 10*time.Second))
		if err != nil && !errors.Is(err, http.ErrNotSupported) {
			return false
		}
		if _, err := fmt.Fprint(w, msg); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send("retry: 5000\n\n") {
		return
	}
	tick := time.NewTicker(s.heartbeat)
	defer tick.Stop()
	for {
		var msg string
		select {
		case <-r.Context().Done():
			return
		case <-s.Done:
			return
		case <-changed:
			msg = "event: week\ndata: changed\n\n"
		case <-tick.C:
			msg = ": ping\n\n"
		}
		if !send(msg) {
			return
		}
	}
}
