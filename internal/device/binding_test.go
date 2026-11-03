package device

import (
	"bytes"
	"regexp"
	"testing"
)

func TestUnit_BindingCodeNormalizationAndEntropy(t *testing.T) {
	code, err := newBindingCode(bytes.NewReader(bytes.Repeat([]byte{0x7f}, 32)))
	if err != nil || !regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{12}$`).MatchString(code) {
		t.Fatalf("generated binding code = %q, %v", code, err)
	}
	if got := normalizeBindingCode("  abcd-1234 "); got != "ABCD1234" {
		t.Fatalf("normalized code = %q", got)
	}
}
