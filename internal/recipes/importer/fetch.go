// Package importer turns a recipe page on the web into a draft matlistan recipe.
package importer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/aldersfors/matlistan/internal/safenet"
)

// Fetch errors; the web layer maps each to a message.
var (
	ErrInvalidURL  = errors.New("not a valid URL")
	ErrNotHTTPS    = errors.New("only https pages can be imported")
	ErrNotAllowed  = safenet.ErrNotAllowed
	ErrUnreachable = safenet.ErrUnreachable
	ErrTooLarge    = errors.New("the page is too large")
	ErrNotHTML     = errors.New("the address is not a web page")
)

const (
	_bodyMax      = 2 << 20
	_redirectsMax = 5
)

// Page is a fetched HTML page; URL is where it ended up after redirects.
type Page struct {
	URL  string
	HTML []byte
}

// Fetcher fetches a recipe page.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string) (Page, error)
}

type safeFetcher struct {
	ua      string
	client  *http.Client
	timeout time.Duration
}

// NewFetcher fetches public https pages only. It resolves each host itself and dials the
// checked address, so neither a DNS answer nor a redirect can reach an internal address.
func NewFetcher(userAgent string) Fetcher {
	return newFetcher(userAgent, safenet.PublicAddr, &http.Transport{
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second,
		MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second, ForceAttemptHTTP2: true,
	})
}

func newFetcher(ua string, allow func(netip.Addr) bool, tr *http.Transport) *safeFetcher {
	f := &safeFetcher{ua: ua, timeout: 10 * time.Second}
	tr.Proxy = nil // never route through an ambient proxy
	tr.DialContext = safenet.Dialer{Allow: allow}.DialContext
	f.client = &http.Client{Transport: tr, CheckRedirect: checkRedirect}
	return f
}

// checkRedirect allows up to _redirectsMax https redirects to hosts safenet allows; the
// dialer then checks the address again.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= _redirectsMax {
		return fmt.Errorf("%w: too many redirects", ErrUnreachable)
	}
	if req.URL.Scheme != "https" {
		return ErrNotHTTPS
	}
	return safenet.CheckHost(req.URL.Hostname())
}

// Fetch implements Fetcher.
func (f *safeFetcher) Fetch(ctx context.Context, rawURL string) (Page, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" || u.User != nil {
		return Page{}, ErrInvalidURL
	}
	switch u.Scheme {
	case "https":
	case "http":
		u.Scheme = "https" // tried once as https; port 80 is closed to the app
	default:
		return Page{}, ErrNotHTTPS
	}
	if err := safenet.CheckHost(u.Hostname()); err != nil {
		return Page{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Page{}, ErrInvalidURL
	}
	req.Header.Set("User-Agent", f.ua)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "sv,en;q=0.5")
	resp, err := f.client.Do(req)
	if err != nil {
		for _, known := range []error{ErrNotAllowed, ErrNotHTTPS} {
			if errors.Is(err, known) {
				return Page{}, known
			}
		}
		return Page{}, fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Page{}, fmt.Errorf("%w: status %d", ErrUnreachable, resp.StatusCode)
	}
	mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if mt != "text/html" && mt != "application/xhtml+xml" {
		return Page{}, ErrNotHTML
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, _bodyMax+1))
	if err != nil {
		return Page{}, fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	if len(body) > _bodyMax {
		return Page{}, ErrTooLarge
	}
	return Page{URL: resp.Request.URL.String(), HTML: body}, nil
}
