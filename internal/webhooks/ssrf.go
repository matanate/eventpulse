package webhooks

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
)

// blockedRanges holds RFC1918 and other special-purpose IP ranges that must
// not receive outbound webhook requests.
// Note: IPv4 CIDRs must be expressed as /8..24; isBlockedIP normalises any
// IPv4-mapped IPv6 address (::ffff:x.x.x.x) to its 4-byte form before checking,
// so we do NOT include ::ffff:0:0/96 here — that would block every IPv4 address.
var blockedRanges = func() []net.IPNet {
	cidrs := []string{
		// IPv4
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"169.254.0.0/16",  // link-local (incl. AWS metadata 169.254.169.254)
		"100.64.0.0/10",   // CGNAT
		"0.0.0.0/8",       // "this" network
		"192.0.2.0/24",    // TEST-NET-1
		"198.51.100.0/24", // TEST-NET-2
		"203.0.113.0/24",  // TEST-NET-3
		// IPv6
		"::1/128",   // loopback (also caught by IsLoopback, belt-and-suspenders)
		"fc00::/7",  // ULA
		"fe80::/10", // link-local
	}
	nets := make([]net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, _ := net.ParseCIDR(c)
		nets = append(nets, *n)
	}
	return nets
}()

// isBlockedIP returns true when ip must not be used as a webhook target.
// IPv4-mapped IPv6 addresses (::ffff:x.x.x.x) are normalised to their 4-byte
// IPv4 form so they are checked against the IPv4 CIDR ranges above.
func isBlockedIP(ip net.IP) bool {
	// Normalise IPv4-in-IPv6 so IPv4 CIDRs match correctly.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	for _, n := range blockedRanges {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// ValidateURL checks that raw is a well-formed URL suitable as a webhook target.
// allowHTTP permits http:// (intended for development/testing only).
func ValidateURL(raw string, allowHTTP bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	switch u.Scheme {
	case "https":
		// always allowed
	case "http":
		if !allowHTTP {
			return errors.New("webhook URL must use https")
		}
	default:
		return errors.New("webhook URL must use https")
	}
	if u.Host == "" {
		return errors.New("webhook URL must have a host")
	}
	// For literal IPs, block private ranges immediately (no DNS needed).
	if ip := net.ParseIP(u.Hostname()); ip != nil && isBlockedIP(ip) {
		return fmt.Errorf("webhook URL host %s is not routable", u.Hostname())
	}
	return nil
}

// safeDial returns a DialContext function that resolves hostnames itself,
// checks every resolved IP against blockedRanges, and then connects directly
// to the checked IP — preventing DNS-rebinding / TOCTOU attacks.
func safeDial(base *net.Dialer) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("split host:port %q: %w", addr, err)
		}

		ips, err := net.DefaultResolver.LookupHost(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("resolve %q: %w", host, err)
		}
		if len(ips) == 0 {
			return nil, fmt.Errorf("no addresses for %q", host)
		}

		for _, a := range ips {
			ip := net.ParseIP(a)
			if ip != nil && isBlockedIP(ip) {
				return nil, fmt.Errorf("SSRF protection: %q resolves to blocked IP %s", host, a)
			}
		}

		// Connect directly to the first resolved IP, bypassing a second DNS
		// round-trip so the checked IP is the one we actually dial.
		return base.DialContext(ctx, network, net.JoinHostPort(ips[0], port))
	}
}
