// Package mailtmpl renders queued transactional mail in both MIME formats.
package mailtmpl

import (
	"bytes"
	_ "embed"
	"html/template"
	"net/url"
	"strings"
	"time"

	"golang.org/x/text/language"
)

const (
	Verification  = "transactional"
	DeviceOffline = "device_offline"
	RefundRequest = "refund_request"
	MailTest      = "mail_test"
)

type Payload struct {
	Locale        string     `json:"locale,omitempty"`
	Purpose       string     `json:"purpose,omitempty"`
	Code          string     `json:"code,omitempty"`
	DeviceName    string     `json:"device_name,omitempty"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	TradeNo       string     `json:"trade_no,omitempty"`

	// Title and Summary belong to previously queued messages.
	Title   map[string]string `json:"title,omitempty"`
	Summary map[string]string `json:"summary,omitempty"`
}

type Message struct {
	Subject string
	Text    string
	HTML    string
}

type detail struct {
	Label string
	Value string
}

type view struct {
	Locale, SiteName, Category, Title, Intro, Code, Note, Action, URL, Footer string
	Details                                                                   []detail
}

//go:embed layout.html
var layout string

var (
	emailLayout = template.Must(template.New("mail").Funcs(template.FuncMap{
		"outlookOpen": func() template.HTML {
			return `<!--[if mso]><table role="presentation" align="center" width="576" cellpadding="0" cellspacing="0" border="0"><tr><td><![endif]-->`
		},
		"outlookClose": func() template.HTML {
			return `<!--[if mso]></td></tr></table><![endif]-->`
		},
	}).Parse(layout))
	headerCleaner = strings.NewReplacer("\r", "", "\n", "")
	utc8          = time.FixedZone("UTC+8", 8*60*60)
)

// RequestLocale uses the first supported, positively weighted language.
func RequestLocale(acceptLanguage string) string {
	tags, _, err := language.ParseAcceptLanguage(acceptLanguage)
	if err == nil {
		for _, tag := range tags {
			base, _ := tag.Base()
			switch base.String() {
			case "en":
				return "en"
			case "zh":
				return "zh-CN"
			}
		}
	}
	return "zh-CN"
}

func Render(name string, payload Payload, siteName, publicBaseURL string) (Message, error) {
	if siteName == "" {
		siteName = "VanillaPay"
	}
	v := newView(name, payload, strings.TrimRight(publicBaseURL, "/"))
	v.SiteName = siteName

	var html bytes.Buffer
	if err := emailLayout.Execute(&html, v); err != nil {
		return Message{}, err
	}
	return Message{
		Subject: headerCleaner.Replace("【" + siteName + "】" + v.Title),
		Text:    v.plainText(),
		HTML:    html.String(),
	}, nil
}

func newView(name string, payload Payload, baseURL string) view {
	english := strings.EqualFold(payload.Locale, "en")
	localize := func(zh, en string) string {
		if english {
			return en
		}
		return zh
	}
	v := view{
		Locale:   localize("zh-CN", "en"),
		Category: localize("系统通知", "System notification"),
		Footer:   localize("此邮件由系统自动发送，请勿直接回复。", "This email was sent automatically. Please do not reply."),
	}
	if len(payload.Title) > 0 || len(payload.Summary) > 0 {
		v.Title = legacyText(payload.Title, english)
		if v.Title == "" {
			v.Title = v.Category
		}
		v.Intro = legacyText(payload.Summary, english)
		v.Code = payload.Code
		return v
	}

	switch name {
	case Verification:
		v.Category = localize("账号安全", "Account security")
		v.Title = verificationTitle(payload.Purpose, localize)
		v.Intro = localize("请在操作页面输入下方验证码。", "Enter the code below to complete your request.")
		v.Code = payload.Code
		v.Note = localize("验证码在 10 分钟内有效。如果这不是你发起的操作，请忽略此邮件。", "This code expires in 10 minutes. If you did not make this request, you can ignore this email.")
	case DeviceOffline:
		v.Category = localize("收款设备", "Payment device")
		v.Title = localize("设备离线", "Device offline")
		v.Intro = localize("收款设备已停止发送心跳。离线期间，顾客无法在收银台完成支付。", "Your payment device has stopped sending heartbeats. Customers cannot pay at checkout while it is offline.")
		if payload.DeviceName != "" {
			v.Details = append(v.Details, detail{localize("设备名称", "Device name"), payload.DeviceName})
		}
		if payload.LastHeartbeat != nil {
			v.Details = append(v.Details, detail{
				localize("最后心跳时间", "Last heartbeat"),
				payload.LastHeartbeat.In(utc8).Format("2006-01-02 15:04:05 MST"),
			})
		}
		v.Note = localize("设备恢复在线并连续在线满 5 分钟后，如再次离线，我们会再次通知你。", "If the device goes offline again after staying online for at least 5 consecutive minutes, we’ll send another alert.")
		v.Action = localize("查看设备", "View device")
		v.URL = baseURL + "/devices"
	case RefundRequest:
		v.Category = localize("订单通知", "Order notification")
		v.Title = localize("退款申请", "Refund request")
		v.Intro = localize("你有一笔新的退款申请。请打开订单查看详情。", "You have a new refund request. Open the order to review the details.")
		if payload.TradeNo != "" {
			v.Details = []detail{{localize("订单号", "Order number"), payload.TradeNo}}
			v.Action = localize("查看订单", "View order")
			v.URL = baseURL + "/orders/" + url.PathEscape(payload.TradeNo)
		}
	case MailTest:
		v.Category = localize("站点设置", "Site settings")
		v.Title = localize("SMTP 测试邮件", "SMTP test email")
		v.Intro = localize("这是一封 SMTP 配置测试邮件。收到此邮件说明站点已通过当前 SMTP 配置成功发送邮件。", "This is an SMTP configuration test. Receiving this message confirms that the site can send email with its current SMTP settings.")
	default:
		v.Title = v.Category
		v.Code = payload.Code
	}
	return v
}

func verificationTitle(purpose string, localize func(zh, en string) string) string {
	switch purpose {
	case "register":
		return localize("注册验证码", "Registration verification code")
	case "reset_password":
		return localize("找回密码验证码", "Password reset verification code")
	case "change_username":
		return localize("修改用户名验证码", "Username change verification code")
	case "change_email_old":
		return localize("验证当前邮箱", "Verify your current email address")
	case "change_email_new":
		return localize("验证新邮箱", "Verify your new email address")
	case "change_password":
		return localize("修改密码验证码", "Password change verification code")
	case "reset_key":
		return localize("重置密钥验证码", "Key reset verification code")
	default:
		return localize("邮箱验证码", "Email verification code")
	}
}

func legacyText(values map[string]string, english bool) string {
	if english && values["en"] != "" {
		return values["en"]
	}
	for _, key := range []string{"zh_cn", "zh_CN", "zh-CN", "en"} {
		if value := values[key]; value != "" {
			return value
		}
	}
	return ""
}

func (v view) plainText() string {
	sections := []string{v.Title}
	if v.Intro != "" {
		sections = append(sections, v.Intro)
	}
	if len(v.Details) > 0 {
		separator := "："
		if v.Locale == "en" {
			separator = ": "
		}
		lines := make([]string, 0, len(v.Details))
		for _, item := range v.Details {
			lines = append(lines, item.Label+separator+item.Value)
		}
		sections = append(sections, strings.Join(lines, "\n"))
	}
	if v.Code != "" {
		sections = append(sections, v.Code)
	}
	if v.Note != "" {
		sections = append(sections, v.Note)
	}
	if v.URL != "" {
		sections = append(sections, v.Action+"\n"+v.URL)
	}
	sections = append(sections, v.SiteName+"\n"+v.Footer)
	return strings.Join(sections, "\n\n") + "\n"
}
