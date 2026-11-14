package mailtmpl

import (
	"encoding/json"
	"html"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const previewBaseURL = "https://pay.example.test"

func TestUnit_MailTemplates(t *testing.T) {
	last := time.Date(2026, 11, 10, 3, 24, 18, 0, time.UTC)
	for _, fixture := range []struct {
		template, file, zh, en, path, bodyValue string
		payload                                 Payload
	}{
		{Verification, "verification", "注册验证码", "Registration verification code", "", "482731", Payload{Purpose: "register", Code: "482731"}},
		{DeviceOffline, "device-offline", "设备离线", "Device offline", "/devices", "2026-11-10 11:24:18 UTC+8", Payload{DeviceName: "前台收款机 / Front counter", LastHeartbeat: &last}},
		{RefundRequest, "refund-request", "退款申请", "Refund request", "/orders/VP20261110001234", "VP20261110001234", Payload{TradeNo: "VP20261110001234"}},
		{RegistrationApproved, "registration-approved", "注册审核已通过", "Registration approved", "/login", "/login", Payload{}},
		{RegistrationRejected, "registration-rejected", "注册审核未通过", "Registration not approved", "", "资料不完整 / Incomplete details", Payload{Reason: "资料不完整 / Incomplete details"}},
		{CallbackFailed, "callback-failed", "异步通知连续失败", "Payment notifications failing", "/orders/VP20261110001234", "VP20261110001234", Payload{TradeNo: "VP20261110001234"}},
		{MailTest, "smtp-test", "SMTP 测试邮件", "SMTP test email", "", "SMTP", Payload{}},
	} {
		for _, locale := range []string{"zh-CN", "en"} {
			t.Run(fixture.file+"/"+locale, func(t *testing.T) {
				payload := fixture.payload
				payload.Locale = locale
				title := fixture.zh
				if locale == "en" {
					title = fixture.en
				}
				msg, err := Render(fixture.template, payload, "VanillaPay", previewBaseURL)
				if err != nil {
					t.Fatal(err)
				}
				if msg.Subject != "【VanillaPay】"+title ||
					!strings.Contains(msg.Text, title+"\n\n") || !strings.Contains(msg.Text, fixture.bodyValue) ||
					!strings.Contains(msg.HTML, `<html lang="`+locale+`">`) || !strings.Contains(html.UnescapeString(msg.HTML), fixture.bodyValue) ||
					!strings.Contains(msg.HTML, "<!--[if mso]>") {
					t.Fatal("unexpected localized subject, plain text or HTML")
				}
				if fixture.path != "" {
					link := previewBaseURL + fixture.path
					if !strings.Contains(msg.HTML, `href="`+link+`"`) || !strings.Contains(msg.Text, link) {
						t.Fatalf("missing absolute URL %s", link)
					}
				} else if strings.Contains(msg.HTML, "href=") {
					t.Fatal("unexpected action link")
				}
				if dir := os.Getenv("MAIL_PREVIEW_DIR"); dir != "" {
					if err := os.MkdirAll(dir, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, fixture.file+"-"+locale+".html"), []byte(msg.HTML), 0o644); err != nil {
						t.Fatal(err)
					}
					t.Logf("\n%s", msg.Text)
				}
			})
		}
	}
}

func TestUnit_MailTemplateEscapesPayloadAndSanitizesSubject(t *testing.T) {
	const hostile = `<img src=x onerror="alert(1)">`
	for _, fixture := range []struct {
		name    string
		payload Payload
	}{
		{DeviceOffline, Payload{DeviceName: hostile}},
		{RefundRequest, Payload{TradeNo: hostile + "/?next=#"}},
		{Verification, Payload{Code: hostile}},
		{"legacy", Payload{Title: map[string]string{"zh_cn": "旧主题\r\nBcc: injected"}, Summary: map[string]string{"zh_cn": hostile}}},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			msg, err := Render(fixture.name, fixture.payload, hostile+"\r\nInjected", previewBaseURL)
			if err != nil {
				t.Fatal(err)
			}
			if strings.ContainsAny(msg.Subject, "\r\n") || strings.Contains(msg.HTML, hostile) ||
				strings.Count(msg.HTML, "&lt;img") < 3 || !strings.Contains(msg.Text, hostile) {
				t.Fatal("payload escaping or subject sanitization failed")
			}
			if fixture.name == RefundRequest && !strings.Contains(msg.HTML, `href="https://pay.example.test/orders/%3Cimg%20src=x%20onerror=%22alert%281%29%22%3E%2F%3Fnext=%23"`) {
				t.Fatal("order number was not escaped as a single URL path segment")
			}
		})
	}
}

func TestUnit_MailTemplateSupportsLegacyPayloads(t *testing.T) {
	for _, fixture := range []struct {
		name, payload, title, content string
	}{
		{Verification, `{"title":{"zh_cn":"Legacy title"},"summary":{"zh_cn":"Legacy body"},"locale":"zh_cn"}`, "Legacy title", "Legacy body"},
		{Verification, `{"code":"482731"}`, "邮箱验证码", "482731"},
		{RefundRequest, `{"title":{"en":"Refund request"},"summary":{"en":"Legacy refund body"},"locale":"en"}`, "Refund request", "Legacy refund body"},
		{MailTest, `{"title":{"zh_CN":"旧版测试"},"summary":{"zh_CN":"旧版正文"}}`, "旧版测试", "旧版正文"},
		{DeviceOffline, `{"device_id":123,"episode":7}`, "设备离线", "收银台"},
	} {
		t.Run(fixture.title, func(t *testing.T) {
			var payload Payload
			if err := json.Unmarshal([]byte(fixture.payload), &payload); err != nil {
				t.Fatal(err)
			}
			msg, err := Render(fixture.name, payload, "Example", previewBaseURL)
			if err != nil || msg.Subject != "【Example】"+fixture.title || !strings.Contains(msg.Text, fixture.content) || !strings.Contains(msg.HTML, fixture.content) {
				t.Fatalf("legacy message did not render: %v", err)
			}
		})
	}
}

func TestUnit_VerificationPurposesAndRequestLocale(t *testing.T) {
	for purpose, expected := range map[string][2]string{
		"register":         {"注册验证码", "Registration verification code"},
		"reset_password":   {"找回密码验证码", "Password reset verification code"},
		"change_username":  {"修改用户名验证码", "Username change verification code"},
		"change_email_old": {"验证当前邮箱", "Verify your current email address"},
		"change_email_new": {"验证新邮箱", "Verify your new email address"},
		"change_password":  {"修改密码验证码", "Password change verification code"},
		"reset_key":        {"重置密钥验证码", "Key reset verification code"},
	} {
		for i, locale := range []string{"zh-CN", "en"} {
			v := newView(Verification, Payload{Purpose: purpose, Locale: locale}, url.URL{})
			if v.Title != expected[i] || !strings.Contains(v.Note, "10") {
				t.Errorf("purpose=%s locale=%s: title=%q note=%q", purpose, locale, v.Title, v.Note)
			}
		}
	}
	for header, want := range map[string]string{
		"": "zh-CN", "fr, en-US;q=0.5": "en", "en;q=0.4, zh-CN;q=0.9": "zh-CN",
		"fr, en;q=0": "zh-CN", "en-GB, zh;q=0.5": "en",
	} {
		if got := RequestLocale(header); got != want {
			t.Errorf("Accept-Language %q: got %q want %q", header, got, want)
		}
	}
}
