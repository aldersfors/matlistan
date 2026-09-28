// Package safenet dials only public internet addresses, so a URL a user or a browser
// supplies cannot make the server reach an internal one.
package safenet

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
)

// Dial errors: ErrNotAllowed for an internal or refused address, ErrUnreachable when the
// name has no address or cannot be looked up.
var (
	ErrNotAllowed  = errors.New("that address is not allowed")
	ErrUnreachable = errors.New("the address could not be reached")
)

var _blockedSuffixes = []string{".svc", ".cluster.local", ".local", ".lan", ".internal"}

// PublicAddr is true for addresses on the public internet.
func PublicAddr(a netip.Addr) bool {
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

// CheckHost refuses names that can only be internal: localhost, single labels and the
// cluster and home-network suffixes.
func CheckHost(host string) error {
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

// Dialer resolves the host itself, refuses the name when any of its addresses is not
// allowed, and connects to a checked address. A nil Allow means PublicAddr.
type Dialer struct{ Allow func(netip.Addr) bool }

// DialContext is for http.Transport.DialContext.
func (d Dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	allow := d.Allow
	if allow == nil {
		allow = PublicAddr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if err := CheckHost(host); err != nil {
		return nil, err
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreachable, err)
	}
	if len(ips) == 0 {
		return nil, ErrUnreachable
	}
	for i, ip := range ips {
		ips[i] = ip.Unmap()
		if !allow(ips[i]) {
			return nil, ErrNotAllowed
		}
	}
	var nd net.Dialer
	return nd.DialContext(ctx, network, net.JoinHostPort(ips[0].String(), port))
}
