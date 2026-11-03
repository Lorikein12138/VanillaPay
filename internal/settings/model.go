package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/mail"
	"net/url"
	"strings"
	"unicode/utf8"
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
	Items   []FooterItem `json:"items"`
	Terms   LanguageText `json:"terms"`
	Privacy LanguageText `json:"privacy"`
}

type mailGroup struct {
	Host        string          `json:"host"`
	Port        int             `json:"port"`
	Username    string          `json:"username"`
	Password    json.RawMessage `json:"password,omitempty"`
	FromAddress string          `json:"from_address"`
	FromName    string          `json:"from_name"`
	TLSMode     string          `json:"tls_mode"`
}

type securityGroup struct {
	TurnstileSiteKey     string          `json:"turnstile_site_key"`
	TurnstileSecret      json.RawMessage `json:"turnstile_secret,omitempty"`
	AllowedHostname      string          `json:"allowed_hostname"`
	RestrictEmailDomains bool            `json:"restrict_email_domains"`
	EmailDomains         []string        `json:"email_domains"`
	BanContact           LanguageText    `json:"ban_contact"`
}

func ValidateGroup(group string, value json.RawMessage) (json.RawMessage, map[string]json.RawMessage, error) {
	if len(value) == 0 || !json.Valid(value) {
		return nil, nil, errors.New("VALIDATION_ERROR")
	}
	secrets := map[string]json.RawMessage{}
	switch group {
	case "site":
		var v struct {
			SiteName    string  `json:"site_name"`
			LogoAssetID *string `json:"logo_asset_id"`
		}
		if err := decodeGroup(value, &v, "site_name", "logo_asset_id"); err != nil || textLength(strings.TrimSpace(v.SiteName)) < 1 || textLength(v.SiteName) > 80 {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		return marshal(v), secrets, nil
	case "home":
		var v struct {
			Mode string  `json:"mode"`
			HTML *string `json:"html"`
			URL  *string `json:"url"`
		}
		if err := decodeGroup(value, &v, "mode", "html", "url"); err != nil {
			return nil, nil, errors.New("VALIDATION_ERROR")
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
		var v mailGroup
		if err := decodeGroup(value, &v, "host", "port", "username", "from_address", "from_name", "tls_mode"); err != nil || textLength(v.Host) < 1 || textLength(v.Host) > 253 || v.Port < 1 || v.Port > 65535 || textLength(v.Username) > 254 || !validEmail(v.FromAddress) || textLength(v.FromName) < 1 || textLength(v.FromName) > 120 || (v.TLSMode != "starttls" && v.TLSMode != "tls") {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		if len(v.Password) > 0 {
			if string(v.Password) != "null" {
				var p string
				if json.Unmarshal(v.Password, &p) != nil || p == "" {
					return nil, nil, errors.New("VALIDATION_ERROR")
				}
				secrets["smtp_password"] = append([]byte(nil), v.Password...)
			} else {
				secrets["smtp_password"] = json.RawMessage("null")
			}
		}
		v.Password = nil
		return marshal(v), secrets, nil
	case "security":
		var v securityGroup
		if err := decodeGroup(value, &v, "turnstile_site_key", "turnstile_secret", "allowed_hostname", "restrict_email_domains", "email_domains", "ban_contact"); err != nil || textLength(v.TurnstileSiteKey) > 512 || textLength(v.AllowedHostname) > 253 || len(v.EmailDomains) > 100 {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		if (v.TurnstileSiteKey == "") != (v.AllowedHostname == "") || (v.RestrictEmailDomains && len(v.EmailDomains) == 0) {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		for i, domain := range v.EmailDomains {
			domain = strings.TrimSpace(strings.ToLower(domain))
			if domain == "" || strings.ContainsAny(domain, " /:@*") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
				return nil, nil, errors.New("VALIDATION_ERROR")
			}
			v.EmailDomains[i] = domain
		}
		if len(v.TurnstileSecret) > 0 {
			if string(v.TurnstileSecret) != "null" {
				var secret string
				if json.Unmarshal(v.TurnstileSecret, &secret) != nil || secret == "" {
					return nil, nil, errors.New("VALIDATION_ERROR")
				}
				secrets["turnstile_secret"] = append([]byte(nil), v.TurnstileSecret...)
			} else {
				secrets["turnstile_secret"] = json.RawMessage("null")
			}
		}
		v.TurnstileSecret = nil
		return marshal(v), secrets, nil
	case "payment":
		var v GroupBounds
		if err := decodeGroup(value, &v, "min_amount_cents", "max_amount_cents", "default_ttl_seconds", "max_ttl_seconds", "float_min_cents", "float_max_cents", "float_default_cents", "float_direction"); err != nil || v.MinAmountCents < 1 || v.MaxAmountCents < v.MinAmountCents || v.MaxAmountCents > hardAmountLimit || v.DefaultTTLSeconds < 60 || v.MaxTTLSeconds < v.DefaultTTLSeconds || v.MaxTTLSeconds > 86400 || v.FloatMinCents < 0 || v.FloatMaxCents < v.FloatMinCents || v.FloatMaxCents > 999 || v.FloatDefaultCents < v.FloatMinCents || v.FloatDefaultCents > v.FloatMaxCents || (v.FloatDirection != "up" && v.FloatDirection != "down") {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		return marshal(v), secrets, nil
	case "billing":
		var v struct {
			MerchantPID string          `json:"merchant_pid"`
			MerchantKey json.RawMessage `json:"merchant_key,omitempty"`
		}
		if err := decodeGroup(value, &v, "merchant_pid"); err != nil || (v.MerchantPID != "" && !validPID(v.MerchantPID)) {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		if len(v.MerchantKey) > 0 {
			if string(v.MerchantKey) != "null" {
				var key string
				if json.Unmarshal(v.MerchantKey, &key) != nil || len(key) != 64 || !isLowerHex(key) {
					return nil, nil, errors.New("VALIDATION_ERROR")
				}
				secrets["billing_key"] = append([]byte(nil), v.MerchantKey...)
			} else {
				secrets["billing_key"] = json.RawMessage("null")
			}
		}
		v.MerchantKey = nil
		return marshal(v), secrets, nil
	case "footer_legal":
		var v FooterLegal
		if err := decodeGroup(value, &v, "items", "terms", "privacy"); err != nil || len(v.Items) > 50 || strings.TrimSpace(v.Terms.ZhCN) == "" || strings.TrimSpace(v.Privacy.ZhCN) == "" || len(v.Terms.ZhCN) > 128<<10 || len(v.Terms.En) > 128<<10 || len(v.Privacy.ZhCN) > 128<<10 || len(v.Privacy.En) > 128<<10 {
			return nil, nil, errors.New("VALIDATION_ERROR")
		}
		seen := map[string]struct{}{}
		for _, item := range v.Items {
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

func decodeGroup(raw json.RawMessage, target any, required ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return errors.New("invalid object")
	}
	for _, field := range required {
		if _, ok := fields[field]; !ok {
			return errors.New("missing field")
		}
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
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
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
