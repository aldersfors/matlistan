package claude

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jalet/matlistan/internal/planner"
)

type fakeAPI struct {
	mu       sync.Mutex // the server goroutine writes bodies, the test reads them
	bodies   []map[string]any
	texts    []string // one reply text per request
	stop     string
	fallback string // when set: this partial text, then a fallback block, then the reply
}

func (f *fakeAPI) body(i int) map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.bodies[i]
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(b, &body)
	f.mu.Lock()
	f.bodies = append(f.bodies, body)
	n := len(f.bodies)
	f.mu.Unlock()
	text := f.texts[min(n, len(f.texts))-1]
	quoted, _ := json.Marshal(text)
	stop := f.stop
	if stop == "" {
		stop = "end_turn"
	}
	w.Header().Set("Content-Type", "text/event-stream")
	events := [][2]string{
		{"message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-opus-5","content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":100,"output_tokens":1,"cache_read_input_tokens":80,"cache_creation_input_tokens":0}}}`},
		{"message_delta", `{"type":"message_delta","delta":{"stop_reason":"` + stop + `","stop_sequence":null},"usage":{"output_tokens":42}}`},
		{"message_stop", `{"type":"message_stop"}`},
	}
	var blocks [][2]string
	index := 0
	textBlock := func(q []byte) {
		i := fmt.Sprint(index)
		blocks = append(blocks,
			[2]string{"content_block_start", `{"type":"content_block_start","index":` + i + `,"content_block":{"type":"text","text":""}}`},
			[2]string{"content_block_delta", `{"type":"content_block_delta","index":` + i + `,"delta":{"type":"text_delta","text":` + string(q) + `}}`},
			[2]string{"content_block_stop", `{"type":"content_block_stop","index":` + i + `}`})
		index++
	}
	if f.fallback != "" {
		partial, _ := json.Marshal(f.fallback)
		textBlock(partial)
		i := fmt.Sprint(index)
		blocks = append(blocks,
			[2]string{"content_block_start", `{"type":"content_block_start","index":` + i + `,"content_block":{"type":"fallback","from":{"model":"claude-opus-5"},"to":{"model":"claude-opus-4-8"},"trigger":{"type":"refusal","category":"cyber"}}}`},
			[2]string{"content_block_stop", `{"type":"content_block_stop","index":` + i + `}`})
		index++
	}
	textBlock(quoted)
	events = append(events[:1], append(blocks, events[1:]...)...)
	for _, e := range events {
		_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e[0], e[1])
	}
}

func newTestClient(t *testing.T, f *fakeAPI, model string) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return New(Config{APIKey: "test-key", Model: model, BaseURL: srv.URL, Timeout: 10 * time.Second})
}

func TestSendBuildsTheRequest(t *testing.T) {
	f := &fakeAPI{texts: []string{`{"days":[]}`, `{"days":[1]}`}}
	s := newTestClient(t, f, DefaultModel).NewSession("SYSTEM", map[string]any{"type": "object"})
	r, err := s.Send(context.Background(), "week 40")
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != `{"days":[]}` || r.Usage.Input != 100 || r.Usage.Output != 42 ||
		r.Usage.CacheRead != 80 {
		t.Fatalf("reply = %+v", r)
	}
	b := f.body(0)
	sys, _ := json.Marshal(b["system"])
	oc, _ := json.Marshal(b["output_config"])
	if b["model"] != DefaultModel || b["stream"] != true || b["fallbacks"] != "default" ||
		!strings.Contains(string(sys), `"cache_control":{"type":"ephemeral"}`) ||
		!strings.Contains(string(oc), `"json_schema"`) {
		t.Fatalf("request = %v", b)
	}
	if _, err := s.Send(context.Background(), "fix it"); err != nil {
		t.Fatal(err)
	}
	msgs, _ := b["messages"].([]any)
	second, _ := f.body(1)["messages"].([]any)
	if len(msgs) != 1 || len(second) != 3 {
		t.Fatalf("history: first %d messages, second %d, want 1 and 3", len(msgs), len(second))
	}
}

func TestNoFallbackForOtherModels(t *testing.T) {
	f := &fakeAPI{texts: []string{`{}`}}
	s := newTestClient(t, f, "claude-sonnet-5").NewSession("S", map[string]any{})
	if _, err := s.Send(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.body(0)["fallbacks"]; ok {
		t.Fatal("fallbacks sent for a model without the feature")
	}
}

func TestStopReasons(t *testing.T) {
	for stop, want := range map[string]error{"refusal": planner.ErrRefused,
		"max_tokens": planner.ErrTruncated} {
		f := &fakeAPI{texts: []string{`{}`}, stop: stop}
		_, err := newTestClient(t, f, DefaultModel).NewSession("S", nil).Send(context.Background(), "x")
		if !errors.Is(err, want) {
			t.Errorf("%s: err = %v", stop, err)
		}
	}
}

func TestAPIErrorIsReturned(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"bad"}}`)
	}))
	t.Cleanup(srv.Close)
	c := New(Config{APIKey: "sk-test-secret-123", Model: DefaultModel, BaseURL: srv.URL,
		Timeout: 5 * time.Second})
	if _, err := c.NewSession("S", nil).Send(context.Background(), "x"); err == nil ||
		strings.Contains(err.Error(), "sk-test-secret-123") {
		t.Fatalf("err = %v", err)
	}
}

// After a mid-answer fallback only the fallback model's text is the answer.
func TestFallbackKeepsOnlyTheFinalAnswer(t *testing.T) {
	f := &fakeAPI{texts: []string{`{"days":[]}`}, fallback: `{"days":[{"da`}
	r, err := newTestClient(t, f, DefaultModel).NewSession("S", nil).Send(context.Background(), "x")
	if err != nil || r.Text != `{"days":[]}` {
		t.Fatalf("reply %q, %v", r.Text, err)
	}
}
