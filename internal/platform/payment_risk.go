package platform

import (
	"errors"
	"net/netip"
	"strings"

	"golang.org/x/net/idna"
	"golang.org/x/text/unicode/norm"
)

const HardAmountLimit int64 = 999999999

type PlanLimits struct {
	MinOrderCents   *int64 `json:"min_order_cents,omitempty"`
	MaxOrderCents   *int64 `json:"max_order_cents,omitempty"`
	DailyLimitCents *int64 `json:"daily_limit_cents,omitempty"`
}

type PaymentRisk struct {
	IPDailyOrderLimit        int32    `json:"ip_daily_order_limit"`
	IPBlacklist              []string `json:"ip_blacklist"`
	NameBlockWords           []string `json:"name_block_words"`
	CheckoutTurnstileEnabled bool     `json:"checkout_turnstile_enabled"`
}

func CheckoutIPBlocked(ip string, rules []string) bool {
	address, err := netip.ParseAddr(ip)
	if err != nil {
		return false
	}
	for _, rule := range rules {
		if prefix, err := netip.ParsePrefix(rule); err == nil &&
			(prefix.Contains(address.Unmap()) || prefix.Contains(netip.AddrFrom16(address.As16()))) {
			return true
		}
	}
	return false
}

func NormalizePaymentName(value string) string {
	return strings.ToLower(norm.NFKC.String(value))
}

func NormalizeIPPrefix(value string) (netip.Prefix, error) {
	value = strings.TrimSpace(value)
	if addr, err := netip.ParseAddr(value); err == nil && addr.Zone() == "" {
		addr = addr.Unmap()
		return netip.PrefixFrom(addr, addr.BitLen()), nil
	}
	prefix, err := netip.ParsePrefix(value)
	if err != nil {
		return netip.Prefix{}, err
	}
	if prefix.Addr().Is4In6() && prefix.Bits() >= 96 {
		prefix = netip.PrefixFrom(prefix.Addr().Unmap(), prefix.Bits()-96)
	}
	return prefix.Masked(), nil
}

func NormalizeNotifyDomain(value string) (string, error) {
	value = strings.TrimSpace(value)
	host, wildcard := strings.CutPrefix(value, "*.")
	host, err := idna.Lookup.ToASCII(host)
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if err != nil || len(host) > 253 {
		return "", errors.New("invalid domain")
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return "", errors.New("invalid domain")
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", errors.New("invalid domain")
		}
		for _, r := range label {
			if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
				return "", errors.New("invalid domain")
			}
		}
	}
	if wildcard {
		return "*." + host, nil
	}
	return host, nil
}

func NotifyDomainAllowed(host string, rules []string) bool {
	host, err := NormalizeNotifyDomain(host)
	if err != nil || strings.HasPrefix(host, "*.") {
		return false
	}
	for _, rule := range rules {
		if suffix, wildcard := strings.CutPrefix(rule, "*"); wildcard {
			if strings.HasSuffix(host, suffix) {
				return true
			}
		} else if host == rule {
			return true
		}
	}
	return false
}
