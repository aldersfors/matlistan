package push

import (
	"errors"
	"strings"
	"testing"
)

func TestCheckEndpoint(t *testing.T) {
	for _, ok := range []string{
		"https://web.push.apple.com/QGuQyavXutnMH-abc",
		"https://fcm.googleapis.com/fcm/send/abc:def",
		"https://updates.push.services.mozilla.com/wpush/v2/gAAA",
		"https://wns2-par02p.notify.windows.com/w/?token=abc",
	} {
		if err := CheckEndpoint(ok); err != nil {
			t.Errorf("%s refused: %v", ok, err)
		}
	}
	for _, bad := range []string{
		"http://web.push.apple.com/x", "https://web.push.apple.com.evil.example/x",
		"https://evil.example/web.push.apple.com", "https://notify.windows.com.evil.example/x",
		"https://127.0.0.1/x", "https://matlistan.example.lan/x", "not a url",
		"https://web.push.apple.com/" + strings.Repeat("a", 1000),
		"https://user@web.push.apple.com/x",
	} {
		if err := CheckEndpoint(bad); !errors.Is(err, ErrEndpoint) {
			t.Errorf("%s accepted: %v", bad, err)
		}
	}
}
