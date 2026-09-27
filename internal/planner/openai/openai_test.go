package openai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aldersfors/matlistan/internal/planner"
)

const _key = "sk-test-0123456789"

type fakeAPI struct {
	mu      sync.Mutex
	bodies  []map[string]any
	auth    []string
	paths   []string
	replies []string // raw JSON response bodies, one per request; the last repeats
	status  int      // when set, every response has this status
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(b, &body)
	f.mu.Lock()
	f.bodies = append(f.bodies, body)
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	f.paths = append(f.paths, r.URL.Path)
	n := len(f.bodies)
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	if f.status != 0 {
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(`{"error":{"message":"nope","type":"invalid_request_error"}}`))
		return
	}
	_, _ = w.Write([]byte(f.replies[min(n, len(f.replies))-1]))
}

func reply(content, finish, refusal string) string {
	b, _ := json.Marshal(map[string]any{"id": "c1", "object": "chat.completion", "created": 1,
		"model": "gpt-test", "choices": []any{map[string]any{"index": 0, "finish_reason": finish,
			"message": map[string]any{"role": "assistant", "content": content, "refusal": refusal}}},
		"usage": map[string]any{"prompt_tokens": 1200, "completion_tokens": 300,
			"total_tokens": 1500, "prompt_tokens_details": map[string]any{"cached_tokens": 1000}}})
	return string(b)
}

func serve(t *testing.T, f *fakeAPI, key string) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return New(Config{APIKey: key, Model: "gpt-test", BaseURL: srv.URL + "/v1",
		Timeout: 5 * time.Second})
}

func TestSendReturnsTextAndUsage(t *testing.T) {
	f := &fakeAPI{replies: []string{reply(`{"days":[]}`, "stop", "")}}
	schema := map[string]any{"type": "object", "additionalProperties": false}
	r, err := serve(t, f, _key).NewSession("be brief", schema).Send(context.Background(), "plan")
	if err != nil || r.Text != `{"days":[]}` {
		t.Fatalf("reply = %+v, %v", r, err)
	}
	if r.Usage != (planner.Usage{Input: 1200, Output: 300, CacheRead: 1000}) {
		t.Errorf("usage = %+v", r.Usage)
	}
	body := f.bodies[0]
	if f.paths[0] != "/v1/chat/completions" || body["model"] != "gpt-test" ||
		body["max_completion_tokens"] != 64000.0 || f.auth[0] != "Bearer "+_key {
		t.Errorf("request = %s %v auth %q", f.paths[0], body, f.auth[0])
	}
	rf, _ := json.Marshal(body["response_format"])
	for _, want := range []string{`"type":"json_schema"`, `"name":"week_plan"`, `"strict":true`,
		`"additionalProperties":false`} {
		if !strings.Contains(string(rf), want) {
			t.Errorf("response_format %s lacks %s", rf, want)
		}
	}
	msgs := body["messages"].([]any)
	if msgs[0].(map[string]any)["role"] != "system" || msgs[1].(map[string]any)["role"] != "user" {
		t.Errorf("messages = %v", msgs)
	}
}

// The planner's retry sends feedback in the same conversation.
func TestSecondSendCarriesTheHistory(t *testing.T) {
	f := &fakeAPI{replies: []string{reply(`{"days":[1]}`, "stop", ""), reply(`{"days":[]}`, "stop", "")}}
	s := serve(t, f, _key).NewSession("sys", map[string]any{})
	if _, err := s.Send(context.Background(), "plan"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Send(context.Background(), "fix day 1"); err != nil {
		t.Fatal(err)
	}
	msgs := f.bodies[1]["messages"].([]any)
	var roles []string
	for _, m := range msgs {
		roles = append(roles, m.(map[string]any)["role"].(string))
	}
	if strings.Join(roles, ",") != "system,user,assistant,user" {
		t.Fatalf("roles = %v", roles)
	}
	if msgs[2].(map[string]any)["content"] != `{"days":[1]}` {
		t.Errorf("assistant turn = %v", msgs[2])
	}
}

func TestRefusalAndTruncation(t *testing.T) {
	for name, c := range map[string]struct {
		body string
		want error
	}{
		"refusal":   {reply("", "stop", "I can't help with that."), planner.ErrRefused},
		"truncated": {reply(`{"days":[`, "length", ""), planner.ErrTruncated},
	} {
		_, err := serve(t, &fakeAPI{replies: []string{c.body}}, _key).NewSession("s", nil).
			Send(context.Background(), "plan")
		if !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
	}
}

// Review focus 3: prose from a server that ignores strict mode goes to the planner
// untouched; the planner's validation decides.
func TestProseReplyReachesThePlannerAsText(t *testing.T) {
	r, err := serve(t, &fakeAPI{replies: []string{reply("Here is your plan!", "stop", "")}}, _key).
		NewSession("s", nil).Send(context.Background(), "plan")
	if err != nil || r.Text != "Here is your plan!" {
		t.Fatalf("reply = %+v, %v", r, err)
	}
	if _, perr := planner.ParseProposal(r.Text); perr == nil {
		t.Fatal("planner accepted prose")
	}
}

func TestEmptyAndFailedReplies(t *testing.T) {
	empty := `{"id":"c","object":"chat.completion","created":1,"model":"m","choices":[]}`
	if _, err := serve(t, &fakeAPI{replies: []string{empty}}, _key).NewSession("s", nil).
		Send(context.Background(), "p"); err == nil || !strings.Contains(err.Error(), "empty reply") {
		t.Errorf("empty choices: %v", err)
	}
	for _, status := range []int{http.StatusUnauthorized, http.StatusInternalServerError} {
		f := &fakeAPI{status: status}
		_, err := serve(t, f, _key).NewSession("s", nil).Send(context.Background(), "p")
		if err == nil || strings.Contains(err.Error(), _key) {
			t.Errorf("status %d: err = %v", status, err)
		}
	}
}

// Review focus 2: ambient OPENAI_* settings neither redirect requests nor add a key.
func TestEnvironmentDoesNotLeakIn(t *testing.T) {
	t.Setenv("OPENAI_BASE_URL", "http://127.0.0.1:1/v1")
	t.Setenv("OPENAI_API_KEY", "sk-from-env")
	f := &fakeAPI{replies: []string{reply("{}", "stop", "")}}
	if _, err := serve(t, f, "").NewSession("s", nil).Send(context.Background(), "p"); err != nil {
		t.Fatalf("request went elsewhere: %v", err)
	}
	if f.auth[0] != "" {
		t.Errorf("keyless client sent Authorization %q", f.auth[0])
	}
}

// Review: after a cut-off answer the planner sends a follow-up in the same conversation;
// the roles must still alternate, or strict chat templates reject the retry.
func TestTruncatedReplyStaysInTheHistory(t *testing.T) {
	f := &fakeAPI{replies: []string{reply(`{"days":[`, "length", ""), reply(`{"days":[]}`, "stop", "")}}
	s := serve(t, f, _key).NewSession("sys", map[string]any{})
	if _, err := s.Send(context.Background(), "plan"); !errors.Is(err, planner.ErrTruncated) {
		t.Fatalf("first send: %v", err)
	}
	if _, err := s.Send(context.Background(), "your answer was cut off"); err != nil {
		t.Fatal(err)
	}
	var roles []string
	for _, m := range f.bodies[1]["messages"].([]any) {
		roles = append(roles, m.(map[string]any)["role"].(string))
	}
	if strings.Join(roles, ",") != "system,user,assistant,user" {
		t.Fatalf("roles = %v", roles)
	}
}

func TestMaxTokensIsConfigurable(t *testing.T) {
	f := &fakeAPI{replies: []string{reply("{}", "stop", "")}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c := New(Config{APIKey: _key, Model: "m", BaseURL: srv.URL + "/v1", Timeout: 5 * time.Second,
		MaxTokens: 16000})
	if _, err := c.NewSession("s", nil).Send(context.Background(), "p"); err != nil {
		t.Fatal(err)
	}
	if f.bodies[0]["max_completion_tokens"] != 16000.0 {
		t.Errorf("max_completion_tokens = %v", f.bodies[0]["max_completion_tokens"])
	}
}
