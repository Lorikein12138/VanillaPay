package payment

import (
	"context"
	"testing"

	"vanillapay/internal/platform"
)

func TestUnit_ProtocolSourceIPNeverUsesClientIP(t *testing.T) {
	if got := protocolSourceIP(context.Background(), "198.51.100.11"); got != "198.51.100.11" {
		t.Fatalf("explicit non-HTTP source = %q", got)
	}
	if got := protocolSourceIP(context.Background(), "not-an-ip"); got != "" {
		t.Fatalf("invalid explicit source = %q", got)
	}
	ctx := platform.WithRequestMetadata(context.Background(), platform.RequestMetadata{IP: "", RequestID: "request"})
	if got := protocolSourceIP(ctx, "198.51.100.11"); got != "" {
		t.Fatalf("missing HTTP source fell back to explicit/client display IP: %q", got)
	}
	ctx = platform.WithRequestMetadata(context.Background(), platform.RequestMetadata{IP: "::ffff:203.0.113.9"})
	if got := protocolSourceIP(ctx, "198.51.100.11"); got != "203.0.113.9" {
		t.Fatalf("HTTP source metadata = %q", got)
	}
}
