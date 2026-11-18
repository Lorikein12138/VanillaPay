package payment

import (
	"html"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnit_SubmitErrorReceipt(t *testing.T) {
	for _, fixture := range []struct {
		name, code, detail string
	}{
		{"risk-detail", "URL_DOMAIN_NOT_ALLOWED", `<section class="detail"><p>商户未允许订单使用的回调或跳转域名。</p><p lang="en">The merchant has not allowed the callback or return domain.</p></section>`},
		{"long-code", "DEMO_PAYMENT_REQUEST_UNAVAILABLE_FOR_THIS_MERCHANT_CONFIGURATION_" + strings.Repeat("X", 80), ""},
		{"escaped-code", `<unexpected>&"`, ""},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			writeSubmitError(w, fixture.code)
			if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "text/html; charset=utf-8" ||
				w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Location") != "" {
				t.Fatalf("unexpected submit error response: %d, %v", w.Code, w.Header())
			}
			body := w.Body.String()
			if !strings.Contains(body, "<code>"+html.EscapeString(fixture.code)+"</code>") ||
				!strings.Contains(body, `<p lang="en">Payment could not start.`) ||
				!strings.Contains(body, "无法发起支付") {
				t.Fatal("missing escaped error code or bilingual instructions")
			}
			if fixture.detail != "" {
				if !strings.Contains(body, fixture.detail) {
					t.Fatal("missing bilingual risk detail")
				}
			} else if strings.Contains(body, `class="detail"`) {
				t.Fatal("rendered an empty risk detail")
			}
			if dir := os.Getenv("PAYMENT_ERROR_PREVIEW_DIR"); dir != "" {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, fixture.name+".html"), w.Body.Bytes(), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
