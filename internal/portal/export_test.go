package portal

import (
	"strings"
	"testing"
)

func TestUnit_ExportCSVFormulaPrefixEscaping(t *testing.T) {
	for _, value := range []string{"=SUM(A1)", "+cmd", "-1", "@SUM(A1)", "\tformula", "\rformula"} {
		if got := csvSafe(value); !strings.HasPrefix(got, "'") {
			t.Errorf("csvSafe(%q) = %q; formula prefix not escaped", value, got)
		}
	}
	for _, value := range []string{"", "safe text", "中文"} {
		if got := csvSafe(value); got != value {
			t.Errorf("csvSafe(%q) = %q", value, got)
		}
	}
}
