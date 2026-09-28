package push

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/aldersfors/matlistan/internal/safenet"
)

// Result is what a push service said about a subscription.
type Result int

// The results: OK when the push service accepted the message, Gone when the device is gone.
const (
	OK   Result = iota // accepted
	Gone               // 404 or 410: the device unsubscribed, delete the row
)

// Sender delivers one encrypted message.
type Sender interface {
	Send(ctx context.Context, sub Subscription, payload []byte) (Result, error)
}

type httpSender struct {
	vapid  *VAPID
	client *http.Client
	check  func(string) error
	now    func() time.Time
}

// NewSender posts to the known push services only, through a dialer that reaches public
// addresses only, with no redirects and a 10 s limit per message.
func NewSender(v *VAPID) Sender {
	tr := &http.Transport{Proxy: nil, DialContext: safenet.Dialer{}.DialContext,
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second,
		MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second, ForceAttemptHTTP2: true}
	return &httpSender{vapid: v, check: CheckEndpoint, now: time.Now,
		client: &http.Client{Transport: tr, Timeout: 10 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

// Send encrypts payload for sub and posts it. Errors name the push service's host only:
// the endpoint works like a password for that device.
func (s *httpSender) Send(ctx context.Context, sub Subscription, payload []byte) (Result, error) {
	host := hostOf(sub.Endpoint)
	if err := s.check(sub.Endpoint); err != nil {
		return 0, fmt.Errorf("push to %s: %w", host, ErrEndpoint)
	}
	body, err := Seal(sub, payload)
	if err != nil {
		return 0, fmt.Errorf("push to %s: %w", host, err)
	}
	auth, err := s.vapid.Header(sub.Endpoint, s.now())
	if err != nil {
		return 0, fmt.Errorf("push to %s: %w", host, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("push to %s: bad request", host)
	}
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", auth)
	res, err := s.client.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // url.Error carries the full URL; keep only the cause
		}
		return 0, fmt.Errorf("push to %s: %w", host, err)
	}
	defer func() { _ = res.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 4096))
	switch {
	case res.StatusCode == http.StatusNotFound || res.StatusCode == http.StatusGone:
		return Gone, nil
	case res.StatusCode >= 200 && res.StatusCode < 300:
		return OK, nil
	default:
		return 0, fmt.Errorf("push to %s: status %d", host, res.StatusCode)
	}
}

func hostOf(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		return u.Hostname()
	}
	return "unknown host"
}
