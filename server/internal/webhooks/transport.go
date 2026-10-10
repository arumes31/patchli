package webhooks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"time"
)

var errNonPublicWebhook = errors.New("webhook destination is not a public address")

// Exclude special-use ranges beyond RFC1918/4193, including shared VPN space
// and IPv6 transition ranges that could carry embedded private IPv4 addresses.
var webhookDeniedNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"), netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"), netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"), netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
}

func publicWebhookIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() || addr.IsLinkLocalUnicast() {
		return false
	}
	if addr.Is6() && !netip.MustParsePrefix("2000::/3").Contains(addr) {
		return false
	}
	for _, prefix := range webhookDeniedNetworks {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

// Resolve once, validate all addresses, then dial an address directly so a DNS
// change cannot turn a validated public hostname into an internal destination.
func dialWebhook(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{Timeout: 10 * time.Second}
	return dialWebhookAddresses(ctx, network, port, addresses, dialer.DialContext)
}

func dialWebhookAddresses(ctx context.Context, network, port string, addresses []net.IPAddr, dial func(context.Context, string, string) (net.Conn, error)) (net.Conn, error) {
	if len(addresses) == 0 {
		return nil, fmt.Errorf("webhook destination has no addresses")
	}
	for _, addr := range addresses {
		if !publicWebhookIP(addr.IP) {
			return nil, errNonPublicWebhook
		}
	}
	var err error
	for _, addr := range addresses {
		conn, dialErr := dial(ctx, network, net.JoinHostPort(addr.IP.String(), port))
		if dialErr == nil {
			return conn, nil
		}
		err = dialErr
	}
	return nil, err
}

var webhookClient = &http.Client{
	Timeout:   10 * time.Second,
	Transport: &http.Transport{DialContext: dialWebhook, TLSHandshakeTimeout: 10 * time.Second},
	// A configured webhook endpoint should not forward payloads to another URL.
	CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
}
