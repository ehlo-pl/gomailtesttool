// Package network provides shared helpers for IPv4/IPv6 address-family
// selection used by the --ipv4/--ipv6 flags across protocol clients.
package network

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// ValidateIPVersionFlags returns an error if both ipv4 and ipv6 are set,
// since they request mutually exclusive address families.
func ValidateIPVersionFlags(ipv4, ipv6 bool) error {
	if ipv4 && ipv6 {
		return fmt.Errorf("cannot use both -ipv4 and -ipv6 flags simultaneously")
	}
	return nil
}

// ResolveForDial returns the host to use when dialing, honoring --ipv4/--ipv6
// by resolving host to a single IPv4 (A record) or IPv6 (AAAA record) literal
// address. If neither flag is set, host is returned unchanged and normal
// dual-stack resolution happens at dial time.
//
// If host is already a literal IP address, it is validated against the
// requested family rather than re-resolved.
//
// Only the first matching address is returned — this pins the connection to
// a single resolved address (useful for diagnostics), not Go's usual
// multi-address dial failover.
func ResolveForDial(ctx context.Context, host string, ipv4, ipv6 bool) (string, error) {
	if !ipv4 && !ipv6 {
		return host, nil
	}

	lookupNetwork, flag, label := "ip4", "-ipv4", "IPv4"
	if ipv6 {
		lookupNetwork, flag, label = "ip6", "-ipv6", "IPv6"
	}

	if ip := net.ParseIP(host); ip != nil {
		isV4 := ip.To4() != nil
		if ipv4 != isV4 {
			return "", fmt.Errorf("%s requested but host %q is not an %s address", flag, host, label)
		}
		return host, nil
	}

	ips, err := net.DefaultResolver.LookupIP(ctx, lookupNetwork, host)
	if err != nil {
		return "", fmt.Errorf("failed to resolve %s address for %q: %w", label, host, err)
	}
	if len(ips) == 0 {
		return "", fmt.Errorf("no %s address found for %q", label, host)
	}
	return ips[0].String(), nil
}

// LookupMX resolves the MX records for domain and returns the hostname of
// the record with the lowest preference value (i.e. the most-preferred mail
// server), with any trailing root-domain dot removed.
func LookupMX(ctx context.Context, domain string) (string, error) {
	records, err := net.DefaultResolver.LookupMX(ctx, domain)
	if err != nil {
		return "", fmt.Errorf("MX lookup for %q failed: %w", domain, err)
	}
	if len(records) == 0 {
		return "", fmt.Errorf("no MX records found for %q", domain)
	}

	best := records[0]
	for _, mx := range records[1:] {
		if mx.Pref < best.Pref {
			best = mx
		}
	}

	return strings.TrimSuffix(best.Host, "."), nil
}
