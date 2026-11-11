package auth

import (
	"strings"
	"unicode/utf8"

	"golang.org/x/net/idna"
)

var (
	domainDots = strings.NewReplacer("。", ".", "．", ".", "｡", ".")
	// A wildcard stands for unknown characters, so the finished email domain
	// is what must pass the bidi check. Literal labels use the lookup profile.
	wildcardLabel = idna.New(idna.MapForLookup(), idna.StrictDomainName(false), idna.CheckHyphens(false))
)

func normalizeEmailDomain(value string) (string, error) {
	domain, err := idna.Lookup.ToASCII(strings.TrimSuffix(domainDots.Replace(value), "."))
	if err != nil || domain == "" || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return "", ErrInvalidEmail
	}
	return domain, nil
}

// NormalizeEmailDomainRule normalizes a rule for storage, rejecting mixed wildcard representations.
func NormalizeEmailDomainRule(value string) (string, error) {
	rule, err := normalizeEmailDomainRule(value)
	if err == nil && !isASCIIDomain(rule) {
		input := domainDots.Replace(strings.ToLower(strings.TrimSpace(value)))
		if strings.Contains("."+input, ".xn--") {
			return "", ErrInvalidEmail
		}
	}
	return rule, err
}

func normalizeEmailDomainRule(value string) (string, error) {
	rule := domainDots.Replace(strings.TrimSpace(value))
	if utf8.RuneCountInString(rule) > 253 {
		return "", ErrInvalidEmail
	}
	if !strings.Contains(rule, "*") {
		normalized, err := idna.Lookup.ToASCII(rule)
		if err != nil || !validRuleDomain(normalized) {
			return "", ErrInvalidEmail
		}
		return normalized, nil
	}
	labels := strings.Split(rule, ".")
	unicodePattern := false
	for i, label := range labels {
		if label == "" {
			return "", ErrInvalidEmail
		}
		normalized, err := normalizeRuleLabel(label)
		if err != nil {
			return "", err
		}
		labels[i] = normalized
		unicodePattern = unicodePattern || !isASCIIDomain(normalized)
	}
	if unicodePattern {
		// A partial Unicode label must be matched in Unicode. Punycode does
		// not preserve the positions of literal characters around a wildcard.
		for i, label := range labels {
			if !strings.Contains(label, "*") {
				normalized, err := idna.Lookup.ToUnicode(label)
				if err != nil {
					return "", err
				}
				labels[i] = normalized
			}
		}
	}
	rule = strings.Join(labels, ".")
	if !validRuleDomain(rule) || strings.Trim(rule, "*") == "" {
		return "", ErrInvalidEmail
	}
	return rule, nil
}

func validRuleDomain(domain string) bool {
	return domain != "" && utf8.RuneCountInString(domain) <= 253 && !strings.HasPrefix(domain, ".") &&
		!strings.HasSuffix(domain, ".") && !strings.Contains(domain, "..")
}

func normalizeRuleLabel(label string) (string, error) {
	normalized, err := idna.Lookup.ToASCII(label)
	if strings.Contains(label, "*") {
		if isASCIIDomain(label) && strings.HasPrefix(strings.ToLower(label), "xn--") {
			// An ASCII wildcard may explicitly match an ACE prefix.
			normalized, err = strings.ToLower(label), nil
		} else {
			normalized, err = wildcardLabel.ToUnicode(label)
		}
	}
	if err != nil || normalized == "" || strings.Count(normalized, "*") != strings.Count(label, "*") {
		return "", ErrInvalidEmail
	}
	for _, r := range normalized {
		if r < utf8.RuneSelf && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '*' {
			return "", ErrInvalidEmail
		}
	}
	return normalized, nil
}

func isASCIIDomain(value string) bool {
	return len(value) == utf8.RuneCountInString(value)
}

func ValidateEmailDomain(email string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	_, domain, ok := strings.Cut(email, "@")
	if !ok {
		return false
	}
	domain, err := normalizeEmailDomain(domain)
	if err != nil {
		return false
	}
	var unicodeDomain string
	for _, value := range allowed {
		rule, err := normalizeEmailDomainRule(strings.TrimPrefix(strings.TrimSpace(value), "@"))
		if err != nil {
			continue
		}
		matchDomain := domain
		if !isASCIIDomain(rule) {
			if unicodeDomain == "" {
				unicodeDomain, err = idna.Lookup.ToUnicode(domain)
				if err != nil {
					continue
				}
			}
			matchDomain = unicodeDomain
		}
		if matchesEmailDomain(matchDomain, rule) {
			return true
		}
	}
	return false
}

func matchesEmailDomain(domain, rule string) bool {
	if !strings.Contains(rule, "*") {
		return domain == rule
	}
	segments := strings.Split(rule, "*")
	remaining, ok := strings.CutPrefix(domain, segments[0])
	if !ok {
		return false
	}
	remaining, ok = strings.CutSuffix(remaining, segments[len(segments)-1])
	if !ok {
		return false
	}
	for _, segment := range segments[1 : len(segments)-1] {
		_, remaining, ok = strings.Cut(remaining, segment)
		if !ok {
			return false
		}
	}
	return true
}
