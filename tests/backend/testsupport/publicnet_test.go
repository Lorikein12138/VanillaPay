package testsupport

import (
	"strings"
	"testing"
)

func TestUnit_PublicNetworkMappingFailsClosed(t *testing.T) {
	mappings := map[string]string{"93.184.216.34:443": "127.0.0.1:49231"}
	physical, err := resolvePublicEndpoint("tcp", "93.184.216.34:443", mappings)
	if err != nil || physical != "127.0.0.1:49231" {
		t.Fatalf("registered logical endpoint resolved to %q, %v", physical, err)
	}
	for _, target := range []struct{ network, address string }{
		{"tcp", "callback.vp.test:443"},
		{"tcp", "93.184.216.35:443"},
		{"tcp", "93.184.216.34:465"},
		{"tcp4", "[2001:db8::1]:443"},
		{"udp", "93.184.216.34:443"},
	} {
		if _, err := resolvePublicEndpoint(target.network, target.address, mappings); err == nil {
			t.Errorf("unregistered endpoint %s|%s was accepted", target.network, target.address)
		}
	}
	if _, err := resolvePublicEndpoint("tcp", "93.184.216.34:443", map[string]string{"93.184.216.34:443": "198.51.100.10:443"}); err == nil || !strings.Contains(err.Error(), "local listener") {
		t.Fatalf("non-loopback physical mapping error = %v", err)
	}
}
