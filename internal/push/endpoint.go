package push

import (
	"errors"
	"net/url"
	"strings"
)

// ErrEndpoint means the URL is not one of the push services the server may post to.
var ErrEndpoint = errors.New("not a supported push endpoint")

const _endpointMax = 1000

var _pushHosts = []string{"web.push.apple.com", "fcm.googleapis.com",
	"updates.push.services.mozilla.com"}

// CheckEndpoint allows https URLs on the known push services only. The browser supplies
// the endpoint, so it is untrusted input the server is about to post to.
func CheckEndpoint(raw string) error {
	if len(raw) > _endpointMax {
		return ErrEndpoint
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return ErrEndpoint
	}
	h := strings.ToLower(u.Hostname())
	for _, allowed := range _pushHosts {
		if h == allowed {
			return nil
		}
	}
	if strings.HasSuffix(h, ".notify.windows.com") {
		return nil
	}
	return ErrEndpoint
}
