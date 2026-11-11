package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/mail"
	"net/url"
	"strings"
	"unicode/utf8"

	"vanillapay/internal/auth"
	"vanillapay/internal/platform"
)

const hardAmountLimit int64 = 999999999

type UpdateGroup struct {
	ExpectedVersion int64           `json:"expected_version"`
	Value           json.RawMessage `json:"value"`
}

type GroupResult struct {
	Version           int64           `json:"version"`
	Group             string          `json:"group"`
	Value             json.RawMessage `json:"value"`
	AffectedMerchants int64           `json:"affected_merchants"`
}

type CollectionSettings struct {
	OrderTTLSeconds *int    `json:"order_ttl_seconds"`
	FloatLimitCents *int    `json:"float_limit_cents"`
	FloatDirection  *string `json:"float_direction"`
	Effective       struct {
		OrderTTLSeconds int    `json:"order_ttl_seconds"`
		FloatLimitCents int    `json:"float_limit_cents"`
		FloatDirection  string `json:"float_direction"`
	} `json:"effective"`
	Bounds struct {
		TTLMaxSeconds int `json:"ttl_max_seconds"`
		FloatMinCents int `json:"float_min_cents"`
		FloatMaxCents int `json:"float_max_cents"`
	} `json:"bounds"`
	Version int64 `json:"version"`
}

type SaveCollectionRequest struct {
	ExpectedVersion int64   `json:"expected_version"`
	OrderTTLSeconds *int    `json:"order_ttl_seconds"`
	FloatLimitCents *int    `json:"float_limit_cents"`
	FloatDirection  *string `json:"float_direction"`
}

type QRDTO struct {
	Channel   string  `json:"channel"`
	AssetID   *string `json:"asset_id"`
	ImageURL  *string `json:"image_url"`
	Enabled   bool    `json:"enabled"`
	UpdatedAt *string `json:"updated_at"`
}

type GroupBounds struct {
	MinAmountCents    int64  `json:"min_amount_cents"`
	MaxAmountCents    int64  `json:"max_amount_cents"`
	DefaultTTLSeconds int    `json:"default_ttl_seconds"`
	MaxTTLSeconds     int    `json:"max_ttl_seconds"`
	FloatMinCents     int    `json:"float_min_cents"`
	FloatMaxCents     int    `json:"float_max_cents"`
	FloatDefaultCents int    `json:"float_default_cents"`
	FloatDirection    string `json:"float_direction"`
}

type LanguageText struct {
	ZhCN string `json:"zh_CN"`
	En   string `json:"en"`
}

type FooterItem struct {
	ID      string       `json:"id"`
	Label   LanguageText `json:"label"`
	Text    string       `json:"text"`
	URL     *string      `json:"url"`
	Enabled bool         `json:"enabled"`
	Sort    int          `json:"sort"`
}

type FooterLegal struct {
	Items   *[]FooterItem `json:"items"`
	Terms   LanguageText  `json:"terms"`
	Privacy LanguageText  `json:"privacy"`
}

type MailGroupRequest struct {
	Host        string  `json:"host"`
	Port        int     `json:"port"`
	Username    string  `json:"username"`
	Password    *string `json:"password,omitempty"`
	FromAddress string  `json:"from_address"`
	FromName    string  `json:"from_name"`
	TLSMode     string  `json:"tls_mode"`
}

type SecurityGroupRequest struct {
	TurnstileSiteKey     string       `json:"turnstile_site_key"`
	TurnstileSecret      *string      `json:"turnstile_secret,omitempty"`
	TurnstileToken       string       `json:"turnstile_token,omitempty"`
	TurnstileEnabled     bool         `json:"turnstile_enabled"`
	RestrictEmailDomains bool         `json:"restrict_email_domains"`
	EmailDomains         *[]string    `json:"email_domains"`
	BanContact           LanguageText `json:"ban_contact"`
}

type SiteGroupRequest struct {
	SiteName       string  `json:"site_name"`
	LogoAssetID    *string `json:"logo_asset_id"`
	FaviconAssetID *string `json:"favicon_asset_id"`
}

type HomeGroupRequest struct {
	Mode string  `json:"mode"`
	HTML *string `json:"html"`
	URL  *string `json:"url"`
}

type BillingGroupRequest struct {
	MerchantPID string  `json:"merchant_pid"`
	MerchantKey *string `json:"merchant_key,omitempty"`
}

type RecoverySecurityRequest struct {
	SiteKey string `json:"turnstile_site_key"`
	Enabled bool   `json:"turnstile_enabled"`
	Secret  string `json:"turnstile_secret,omitempty"`
}

func ValidateGroup(group string, value json.RawMessage) (json.RawMessage, map[string]json.RawMessage, error) {
	if len(value) == 0 || !json.Valid(value) {
		return nil, nil, errors.New("VALIDATION_ERROR")
	}
	secrets := map[string]json.RawMessage{}
	switch group {
	case "site":
		var v SiteGroupRequest
		if err := decodeGroup(value, &v); err != nil {
			return nil, nil, err
		}
		if textLength(strings.TrimSpace(v.SiteName)) < 1 || textLength(v.SiteName) > 80 {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		return marshal(v), secrets, nil
	case "home":
		var v HomeGroupRequest
		if err := decodeGroup(value, &v); err != nil {
			return nil, nil, err
		}
		switch v.Mode {
		case "default":
			if v.HTML != nil || v.URL != nil {
				return nil, nil, errors.New("VALIDATION_ERROR")
			}
		case "html":
			if v.HTML == nil || *v.HTML == "" || len([]byte(*v.HTML)) > 256<<10 || v.URL != nil {
				return nil, nil, errors.New("VALIDATION_ERROR")
			}
		case "url":
			if v.URL == nil || v.HTML != nil || !validPublicHTTPSURL(*v.URL) || len(*v.URL) > 2048 {
				return nil, nil, errors.New("VALIDATION_ERROR")
			}
		default:
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		return marshal(v), secrets, nil
	case "mail":
		var v MailGroupRequest
		if err := decodeGroup(value, &v); err != nil {
			return nil, nil, err
		}
		if textLength(v.Host) < 1 || textLength(v.Host) > 253 || v.Port < 1 || v.Port > 65535 || textLength(v.Username) > 254 || !validEmail(v.FromAddress) || textLength(v.FromName) < 1 || textLength(v.FromName) > 120 || (v.TLSMode != "starttls" && v.TLSMode != "tls") {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		if v.Password != nil && *v.Password == "" {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		if password := groupField(value, "password"); password != nil {
			secrets["smtp_password"] = password
		}
		v.Password = nil
		return marshal(v), secrets, nil
	case "security":
		var v SecurityGroupRequest
		if err := decodeGroup(value, &v); err != nil {
			return nil, nil, err
		}
		var domains []string
		if v.EmailDomains != nil {
			domains = *v.EmailDomains
		}
		if textLength(v.TurnstileSiteKey) > 512 || len(domains) > 100 {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		if (v.TurnstileEnabled && strings.TrimSpace(v.TurnstileSiteKey) == "") || (v.RestrictEmailDomains && len(domains) == 0) {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		unique := make(map[string]struct{}, len(domains))
		normalized := domains[:0]
		for _, domain := range domains {
			domain, err := auth.NormalizeEmailDomainRule(domain)
			if err != nil {
				return nil, nil, &platform.Error{Code: "VALIDATION_ERROR", HTTPStatus: 400, Fields: map[string]string{"email_domains": "invalid_domain_rule"}}
			}
			if _, exists := unique[domain]; !exists {
				unique[domain] = struct{}{}
				normalized = append(normalized, domain)
			}
		}
		if v.EmailDomains != nil {
			v.EmailDomains = &normalized
		}
		if v.TurnstileSecret != nil && strings.TrimSpace(*v.TurnstileSecret) == "" {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		if secret := groupField(value, "turnstile_secret"); secret != nil {
			secrets["turnstile_secret"] = secret
		}
		v.TurnstileSecret = nil
		v.TurnstileToken = ""
		return marshal(v), secrets, nil
	case "payment":
		var v GroupBounds
		if err := decodeGroup(value, &v); err != nil {
			return nil, nil, err
		}
		if v.MinAmountCents < 1 || v.MaxAmountCents < v.MinAmountCents || v.MaxAmountCents > hardAmountLimit || v.DefaultTTLSeconds < 60 || v.MaxTTLSeconds < v.DefaultTTLSeconds || v.MaxTTLSeconds > 86400 || v.FloatMinCents < 0 || v.FloatMaxCents < v.FloatMinCents || v.FloatMaxCents > 999 || v.FloatDefaultCents < v.FloatMinCents || v.FloatDefaultCents > v.FloatMaxCents || (v.FloatDirection != "up" && v.FloatDirection != "down") {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		return marshal(v), secrets, nil
	case "billing":
		var v BillingGroupRequest
		if err := decodeGroup(value, &v); err != nil {
			return nil, nil, err
		}
		if v.MerchantPID != "" && !validPID(v.MerchantPID) {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		if v.MerchantKey != nil && (len(*v.MerchantKey) != 64 || !isLowerHex(*v.MerchantKey)) {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		if key := groupField(value, "merchant_key"); key != nil {
			secrets["billing_key"] = key
		}
		v.MerchantKey = nil
		return marshal(v), secrets, nil
	case "footer_legal":
		var v FooterLegal
		if err := decodeGroup(value, &v); err != nil {
			return nil, nil, err
		}
		var items []FooterItem
		if v.Items != nil {
			items = *v.Items
		}
		if len(items) > 50 || strings.TrimSpace(v.Terms.ZhCN) == "" || strings.TrimSpace(v.Privacy.ZhCN) == "" || len(v.Terms.ZhCN) > 128<<10 || len(v.Terms.En) > 128<<10 || len(v.Privacy.ZhCN) > 128<<10 || len(v.Privacy.En) > 128<<10 {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		seen := map[string]struct{}{}
		for _, item := range items {
			if item.ID == "" || len(item.ID) > 64 || item.Sort < 0 || textLength(item.Label.ZhCN) == 0 || textLength(item.Label.ZhCN) > 80 || textLength(item.Label.En) > 80 || textLength(item.Text) > 1000 {
				return nil, nil, errors.New("VALIDATION_ERROR")
			}
			if _, ok := seen[item.ID]; ok {
				return nil, nil, errors.New("VALIDATION_ERROR")
			}
			seen[item.ID] = struct{}{}
			if item.URL != nil && !validFooterURL(*item.URL) {
				return nil, nil, errors.New("VALIDATION_ERROR")
			}
		}
		return marshal(v), secrets, nil
	default:
		return nil, nil, errors.New("VALIDATION_ERROR")
	}
}

func decodeGroup(raw json.RawMessage, target any) error {
	err := platform.DecodeStrictJSON(bytes.NewReader(raw), target)
	var problem *platform.Error
	if errors.As(err, &problem) && len(problem.Fields) == 1 {
		for field := range problem.Fields {
			return platform.InvalidJSONField("value." + field)
		}
	}
	return err
}

// Called only after strict decoding. The raw value distinguishes omission
// (preserve the secret) from an explicit null (clear it).
func groupField(raw json.RawMessage, field string) json.RawMessage {
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	return fields[field]
}

func marshal(value any) json.RawMessage { b, _ := json.Marshal(value); return b }
func textLength(value string) int       { return utf8.RuneCountInString(value) }
func validPublicHTTPSURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.Fragment == "" && u.Opaque == ""
}
func validFooterURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.User != nil || u.Fragment != "" {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" || u.Scheme == "mailto" && u.Opaque != "" && !strings.ContainsAny(u.Opaque, "\r\n")
}
func validEmail(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == value && !strings.ContainsAny(value, "\r\n")
}
func validPID(value string) bool {
	if len(value) != 16 || value[0] == '0' {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func isLowerHex(value string) bool {
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

type MailTestRequest struct {
	Recipient string `json:"recipient"`
}

type SetQREnabledRequest struct {
	Enabled bool `json:"enabled"`
}
