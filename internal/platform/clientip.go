package platform

import (
	"net"
	"net/netip"
	"strings"
)

// ResolveClientIP returns the address observed at the application boundary.
// Forwarded addresses are considered only when the direct peer is trusted.
func ResolveClientIP(remoteAddr, forwardedFor string, trusted []netip.Prefix) netip.Addr {
	peer, err := parsePeer(remoteAddr)
	if err != nil || peer.Zone() != "" {
		return netip.Addr{}
	}
	peer = peer.Unmap()
	if strings.TrimSpace(forwardedFor) == "" || !isTrusted(peer, trusted) {
		return peer
	}
	parts := strings.Split(forwardedFor, ",")
	current := peer
	for index := len(parts) - 1; index >= 0; index-- {
		if !isTrusted(current, trusted) {
			break
		}
		candidate, err := netip.ParseAddr(strings.TrimSpace(parts[index]))
		if err != nil || candidate.Zone() != "" {
			return peer
		}
		current = candidate.Unmap()
	}
	return current
}

func parsePeer(value string) (netip.Addr, error) {
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	return netip.ParseAddr(strings.TrimSpace(value))
}

func isTrusted(address netip.Addr, trusted []netip.Prefix) bool {
	for _, prefix := range trusted {
		if prefix.Contains(address) {
			return true
		}
		if prefix.Addr().Is4In6() && address.Is4() && prefix.Bits() >= 96 {
			bits := prefix.Bits() - 96
			if bits >= 0 && netip.PrefixFrom(prefix.Addr().Unmap(), bits).Masked().Contains(address) {
				return true
			}
		}
	}
	return false
}

func normalizeAuditIP(value string) string {
	address, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || address.Zone() != "" {
		return ""
	}
	return address.Unmap().String()
}
