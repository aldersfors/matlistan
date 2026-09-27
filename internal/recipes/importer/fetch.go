// Package importer turns a recipe page on the web into a draft matlistan recipe.
package importer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

// Fetch errors; the web layer maps each to a message.
var (
	ErrInvalidURL  = errors.New("not a valid URL")
	ErrNotHTTPS    = errors.New("only https pages can be imported")
	ErrNotAllowed  = errors.New("that address is not allowed")
	ErrUnreachable = errors.New("the page could not be fetched")
	ErrTooLarge    = errors.New("the page is too large")
	ErrNotHTML     = errors.New("the address is not a web page")
)

const (
	_bodyMax      = 2 << 20
	_redirectsMax = 5
)

var _blockedSuffixes = []string{".svc", ".cluster.local", ".local", ".lan", ".internal"}

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
	allow   func(netip.Addr) bool
	client  *http.Client
	timeout time.Duration
}

// NewFetcher fetches public https pages only. It resolves each host itself and dials the
// checked address, so neither a DNS answer nor a redirect can reach an internal address.
func NewFetcher(userAgent string) Fetcher {
	return newFetcher(userAgent, publicAddr, &http.Transport{
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second,
		MaxIdleConns: 4, IdleConnTimeout: 30 * time.Second, ForceAttemptHTTP2: true,
	})
}

func newFetcher(ua string, allow func(netip.Addr) bool, tr *http.Transport) *safeFetcher {
	f := &safeFetcher{ua: ua, allow: allow, timeout: 10 * time.Second}
	tr.Proxy = nil // never route through an ambient proxy
	tr.DialContext = f.dial
	f.client = &http.Client{Transport: tr, CheckRedirect: checkRedirect}
	return f
}

// checkRedirect allows up to _redirectsMax https redirects to hosts checkHost allows; the
// dialer then checks the address again.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= _redirectsMax {
		return fmt.Errorf("%w: too many redirects", ErrUnreachable)
	}
	if req.URL.Scheme != "https" {
		return ErrNotHTTPS
	}
	return checkHost(req.URL.Hostname())
}

// publicAddr is true for addresses on the public internet.
func publicAddr(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() ||
		a.IsLinkLocalMulticast() || a.IsMulticast() || a.IsUnspecified() ||
		a.IsInterfaceLocalMulticast() {
		return false
	}
	return !netip.MustParsePrefix("100.64.0.0/10").Contains(a) &&
		!netip.MustParsePrefix("192.0.0.0/24").Contains(a) &&
		!netip.MustParsePrefix("198.18.0.0/15").Contains(a)
}

func checkHost(host string) error {
	h := strings.ToLower(strings.TrimSuffix(host, "."))
	if h == "" || h == "localhost" || !strings.Contains(h, ".") && net.ParseIP(h) == nil {
		return ErrNotAllowed
	}
	for _, s := range _blockedSuffixes {
		if strings.HasSuffix(h, s) {
			return ErrNotAllowed
		}
	}
	return nil
}

// dial resolves the host, refuses non-public addresses and connects to a checked address.
func (f *safeFetcher) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if err := checkHost(host); err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	if len(ips) == 0 {
		return nil, ErrUnreachable
	}
	// Every address the name has must be public: a name that also points inside is refused.
	for i, ip := range ips {
		ips[i] = ip.Unmap()
		if !f.allow(ips[i]) {
			return nil, ErrNotAllowed
		}
	}
	var d net.Dialer
	return d.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
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
	if err := checkHost(u.Hostname()); err != nil {
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
