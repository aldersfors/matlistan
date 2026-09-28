package push

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testVAPID(t *testing.T) *VAPID {
	t.Helper()
	priv, _, _ := GenerateKey()
	v, err := ParseKey(priv, "https://matlistan.example.org")
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func testSub(t *testing.T, endpoint string) Subscription {
	t.Helper()
	return Subscription{Endpoint: endpoint,
		P256DH: b64(t, "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4"),
		Auth:   b64(t, "BTBZMqHH6r4Tts7J_aSIgg")}
}

// testSender posts to a local test server: the test replaces the safe client and the
// endpoint check, which production never does.
func testSender(t *testing.T, srv *httptest.Server, timeout time.Duration) *httpSender {
	t.Helper()
	c := srv.Client()
	c.Timeout = timeout
	return &httpSender{vapid: testVAPID(t), client: c, check: func(string) error { return nil },
		now: time.Now}
}

func TestSenderSendsTheRequiredHeaders(t *testing.T) {
	var got *http.Request
	var body []byte
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, body = r, must(io.ReadAll(r.Body))
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()
	res, err := testSender(t, srv, time.Second).Send(t.Context(), testSub(t, srv.URL+"/push/abc"), []byte(`{"title":"x"}`))
	if err != nil || res != OK {
		t.Fatalf("Send = %v, %v", res, err)
	}
	for k, want := range map[string]string{"Content-Encoding": "aes128gcm",
		"Content-Type": "application/octet-stream", "Ttl": "86400", "Urgency": "normal"} {
		if got.Header.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, got.Header.Get(k), want)
		}
	}
	if !strings.HasPrefix(got.Header.Get("Authorization"), "vapid t=") || len(body) < 86 {
		t.Errorf("auth %q, body %d bytes", got.Header.Get("Authorization"), len(body))
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestSenderResults(t *testing.T) {
	for code, want := range map[int]struct {
		res Result
		err bool
	}{http.StatusCreated: {OK, false}, http.StatusOK: {OK, false},
		http.StatusNotFound: {Gone, false}, http.StatusGone: {Gone, false},
		http.StatusForbidden: {0, true}, http.StatusTooManyRequests: {0, true},
		http.StatusInternalServerError: {0, true}} {
		srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(code)
		}))
		sub := testSub(t, srv.URL+"/push/secret-token")
		res, err := testSender(t, srv, time.Second).Send(t.Context(), sub, []byte("x"))
		srv.Close()
		if (err != nil) != want.err || !want.err && res != want.res {
			t.Errorf("%d: Send = %v, %v", code, res, err)
		}
		if err != nil && strings.Contains(err.Error(), "secret-token") {
			t.Errorf("%d: error leaks the endpoint: %v", code, err)
		}
	}
}

// Review focus 3: a hanging push service costs one timeout, then Send returns.
func TestSenderTimesOut(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	start := time.Now()
	_, err := testSender(t, srv, 100*time.Millisecond).Send(context.Background(), testSub(t, srv.URL+"/p"), []byte("x"))
	if err == nil || time.Since(start) > 2*time.Second {
		t.Fatalf("Send = %v after %v", err, time.Since(start))
	}
}

func TestNewSenderRefusesUnlistedHosts(t *testing.T) {
	s := NewSender(testVAPID(t))
	for _, e := range []string{"https://127.0.0.1/p", "https://evil.example/p"} {
		if _, err := s.Send(t.Context(), testSub(t, e), []byte("x")); err == nil {
			t.Errorf("%s: sent", e)
		}
	}
}
