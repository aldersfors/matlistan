package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/aldersfors/matlistan/internal/push"
)

func sub(n int) push.Subscription {
	p := make([]byte, 65)
	p[0] = 4
	return push.Subscription{Endpoint: fmt.Sprintf("https://web.push.apple.com/dev-%d", n),
		P256DH: p, Auth: bytes.Repeat([]byte{byte(n)}, 16)}
}

// Review focus 5: the same endpoint again updates the row; the limit counts devices.
func TestPushSubscriptionUpsertAndLimit(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	for i := range 20 {
		if err := s.SavePushSubscription(ctx, "sub-anna", sub(i)); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}
	again := sub(3)
	again.Auth = bytes.Repeat([]byte{9}, 16)
	if err := s.SavePushSubscription(ctx, "sub-erik", again); err != nil {
		t.Fatalf("update at the limit: %v", err)
	}
	if n, _ := s.CountPushSubscriptions(ctx); n != 20 {
		t.Fatalf("count = %d", n)
	}
	if err := s.SavePushSubscription(ctx, "sub-anna", sub(20)); !errors.Is(err, ErrTooMany) {
		t.Fatalf("21st: %v", err)
	}
	list, err := s.ListPushSubscriptions(ctx)
	if err != nil || len(list) != 20 {
		t.Fatalf("list = %d, %v", len(list), err)
	}
	for _, l := range list {
		if l.Endpoint == again.Endpoint && !bytes.Equal(l.Auth, again.Auth) {
			t.Error("update did not replace the keys")
		}
	}
}

func TestPushSubscriptionDeleteAndMark(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	if err := s.SavePushSubscription(ctx, "sub-anna", sub(1)); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkPushSent(ctx, sub(1).Endpoint, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePushSubscription(ctx, sub(1).Endpoint); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePushSubscription(ctx, sub(1).Endpoint); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestPushSubscriptionChecks(t *testing.T) {
	s, ctx := newTestStore(t), context.Background()
	bad := sub(1)
	bad.Auth = []byte{1}
	if err := s.SavePushSubscription(ctx, "sub-anna", bad); err == nil {
		t.Fatal("short auth stored")
	}
}
