package safenet

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func TestPublicAddr(t *testing.T) {
	for addr, want := range map[string]bool{
		"93.184.216.34": true, "2606:2800:220:1::1": true,
		"127.0.0.1": false, "10.1.2.3": false, "192.168.1.1": false, "169.254.169.254": false,
		"100.64.0.1": false, "::1": false, "fd00::1": false, "0.0.0.0": false,
		"::ffff:10.0.0.1": false, "198.18.0.1": false,
	} {
		if got := PublicAddr(netip.MustParseAddr(addr)); got != want {
			t.Errorf("PublicAddr(%s) = %v, want %v", addr, got, want)
		}
	}
}

func TestCheckHost(t *testing.T) {
	for _, h := range []string{"", "localhost", "router", "db.svc", "x.cluster.local",
		"nas.local", "matlistan.lan", "a.internal", "LOCALHOST."} {
		if err := CheckHost(h); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("CheckHost(%q) = %v", h, err)
		}
	}
	if err := CheckHost("web.push.apple.com"); err != nil {
		t.Errorf("public host refused: %v", err)
	}
}

func TestDialerRefusesPrivateAnswers(t *testing.T) {
	d := Dialer{}
	if _, err := d.DialContext(context.Background(), "tcp", "127.0.0.1:443"); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("loopback dial: %v", err)
	}
	if _, err := d.DialContext(context.Background(), "tcp", "localhost:443"); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("localhost dial: %v", err)
	}
}
