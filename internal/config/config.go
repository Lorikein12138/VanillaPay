package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
)

// Config contains process configuration. EncryptionKey is never included in
// its String or GoString representation.
type Config struct {
	AppEnv            string
	DatabaseURL       string
	PublicBaseURL     string
	EncryptionKey     [32]byte
	ListenAddr        string
	DataDir           string
	TrustedProxyCIDRs []netip.Prefix
}

// String returns a safe summary that can be used in diagnostics.
func (Config) String() string {
	return "Config{AppEnv:<set>, DatabaseURL:<redacted>, PublicBaseURL:<set>, EncryptionKey:<redacted>, ListenAddr:<set>, DataDir:<set>, TrustedProxyCIDRs:<set>}"
}

// GoString prevents %#v formatting from exposing the encryption key or DSN.
func (c Config) GoString() string {
	return c.String()
}

// ConfigError identifies invalid environment variable names without including
// their values.
type ConfigError struct {
	Names []string
}

func (e *ConfigError) Error() string {
	names := append([]string(nil), e.Names...)
	sort.Strings(names)
	return "invalid configuration: " + strings.Join(names, ", ")
}

// Load reads and validates the required process environment.
func Load() (Config, error) {
	return load(os.LookupEnv)
}

func load(lookup func(string) (string, bool)) (Config, error) {
	var cfg Config
	var invalid []string

	readRequired := func(name string) string {
		value, ok := lookup(name)
		if !ok || strings.TrimSpace(value) == "" {
			invalid = append(invalid, name)
			return ""
		}
		return strings.TrimSpace(value)
	}

	cfg.DatabaseURL = readRequired("DATABASE_URL")
	cfg.PublicBaseURL = readRequired("PUBLIC_BASE_URL")
	encodedKey := readRequired("APP_ENCRYPTION_KEY")
	cfg.ListenAddr = readRequired("LISTEN_ADDR")
	cfg.DataDir = readRequired("DATA_DIR")
	trustedProxyValue, hasTrustedProxyValue := lookup("TRUSTED_PROXY_CIDRS")
	if !hasTrustedProxyValue {
		invalid = append(invalid, "TRUSTED_PROXY_CIDRS")
	}

	appEnv, hasAppEnv := lookup("APP_ENV")
	if !hasAppEnv || strings.TrimSpace(appEnv) == "" {
		cfg.AppEnv = "production"
	} else {
		cfg.AppEnv = strings.ToLower(strings.TrimSpace(appEnv))
		if cfg.AppEnv != "development" && cfg.AppEnv != "production" {
			invalid = append(invalid, "APP_ENV")
		}
	}

	if cfg.DatabaseURL != "" && !validDatabaseURL(cfg.DatabaseURL) {
		invalid = append(invalid, "DATABASE_URL")
	}
	if cfg.PublicBaseURL != "" && !validPublicBaseURL(cfg.AppEnv, cfg.PublicBaseURL) {
		invalid = append(invalid, "PUBLIC_BASE_URL")
	}
	if encodedKey != "" {
		key, err := base64.StdEncoding.DecodeString(encodedKey)
		if err != nil || len(key) != len(cfg.EncryptionKey) {
			invalid = append(invalid, "APP_ENCRYPTION_KEY")
		} else {
			copy(cfg.EncryptionKey[:], key)
		}
	}
	if cfg.ListenAddr != "" && !validListenAddr(cfg.ListenAddr) {
		invalid = append(invalid, "LISTEN_ADDR")
	}
	if trustedProxyValue != "" {
		prefixes, err := parseTrustedProxyCIDRs(trustedProxyValue)
		if err != nil {
			invalid = append(invalid, "TRUSTED_PROXY_CIDRS")
		} else {
			cfg.TrustedProxyCIDRs = prefixes
		}
	}

	if len(invalid) > 0 {
		return Config{}, &ConfigError{Names: unique(invalid)}
	}
	return cfg, nil
}

func validDatabaseURL(value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.Fragment != "" {
		return false
	}
	return (u.Scheme == "postgres" || u.Scheme == "postgresql") && u.Fragment == ""
}

func validPublicBaseURL(appEnv, value string) bool {
	u, err := url.Parse(value)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return false
	}
	if appEnv == "development" && u.Scheme == "http" {
		return isLoopbackHost(u.Hostname())
	}
	return u.Scheme == "https"
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address, err := netip.ParseAddr(host)
	return err == nil && address.IsLoopback()
}

func validListenAddr(value string) bool {
	host, portValue, err := net.SplitHostPort(value)
	if err != nil {
		return false
	}
	port, err := strconv.Atoi(portValue)
	if err != nil || port < 1 || port > 65535 {
		return false
	}
	if host == "" {
		return true
	}
	return net.ParseIP(host) != nil || strings.TrimSpace(host) == host
}

func parseTrustedProxyCIDRs(value string) ([]netip.Prefix, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	parts := strings.Split(value, ",")
	prefixes := make([]netip.Prefix, 0, len(parts))
	seen := make(map[netip.Prefix]struct{}, len(parts))
	for _, part := range parts {
		prefix, err := netip.ParsePrefix(strings.TrimSpace(part))
		if err != nil {
			return nil, fmt.Errorf("invalid prefix")
		}
		prefix = prefix.Masked()
		if _, ok := seen[prefix]; ok {
			continue
		}
		seen[prefix] = struct{}{}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}
