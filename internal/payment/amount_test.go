package payment

import "testing"

func TestUnit_ParseCentsStrictBoundaries(t *testing.T) {
	for _, raw := range []string{"1e2", ".50", "1.", "1.000", "-0.01", "10000000.00"} {
		if _, err := ParseCents(raw); err == nil {
			t.Errorf("ParseCents(%q) unexpectedly succeeded", raw)
		}
	}
	if got, err := ParseCents("9999999.99"); err != nil || got != MaxAmountCents {
		t.Fatalf("maximum amount = %d, %v", got, err)
	}
}
