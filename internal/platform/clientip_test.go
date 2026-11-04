package platform

import (
	"net/netip"
	"testing"
)

func TestUnit_ClientIPTrustChain(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8"), netip.MustParsePrefix("2001:db8:ffff::/48")}
	tests := []struct {
		name, peer, forwarded string
		trusted               []netip.Prefix
		want                  string
	}{
		{name: "untrusted peer ignores spoofed header", peer: "192.0.2.4:443", forwarded: "198.51.100.8", trusted: trusted, want: "192.0.2.4"},
		{name: "no trusted proxies uses peer", peer: "192.0.2.4:443", forwarded: "198.51.100.8", want: "192.0.2.4"},
		{name: "trusted peer resolves right to left", peer: "10.0.0.3:443", forwarded: "198.51.100.8, 10.0.0.2", trusted: trusted, want: "198.51.100.8"},
		{name: "invalid forwarded member falls back to peer", peer: "10.0.0.3:443", forwarded: "198.51.100.8, invalid", trusted: trusted, want: "10.0.0.3"},
		{name: "mapped peer unmaps", peer: "[::ffff:192.0.2.4]:443", want: "192.0.2.4"},
		{name: "mapped trusted prefix", peer: "[::ffff:10.0.0.3]:443", forwarded: "198.51.100.8", trusted: []netip.Prefix{netip.MustParsePrefix("::ffff:10.0.0.0/104")}, want: "198.51.100.8"},
		{name: "IPv6 trusted chain", peer: "[2001:db8:ffff::2]:443", forwarded: "2001:db8::9, 2001:db8:ffff::1", trusted: trusted, want: "2001:db8::9"},
		{name: "zone peer rejected", peer: "[fe80::1%eth0]:443", forwarded: "", trusted: trusted, want: ""},
		{name: "malformed peer rejected", peer: "not-an-ip:443", forwarded: "198.51.100.8", trusted: trusted, want: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ResolveClientIP(test.peer, test.forwarded, test.trusted)
			if test.want == "" {
				if got.IsValid() {
					t.Fatalf("expected invalid address, got %s", got)
				}
				return
			}
			if !got.IsValid() || got.String() != test.want {
				t.Fatalf("ResolveClientIP() = %v, want %s", got, test.want)
			}
		})
	}
}
