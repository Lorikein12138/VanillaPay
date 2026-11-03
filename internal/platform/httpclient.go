package platform

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/idna"
)

var ErrOutboundHTTPUnavailable = errors.New("safe outbound HTTP is unavailable")

// SafeHTTPClient is the outbound HTTP port. Implementations must enforce the
// public-address, redirect, timeout, and response-size rules in the design.
type SafeHTTPClient interface {
	DoPublic(context.Context, *http.Request) (*http.Response, error)
}

// ContextDialer establishes only the raw TCP connection. The HTTP transport
// remains responsible for TLS, hostname verification, and HTTP semantics.
type ContextDialer interface {
	DialContext(context.Context, string, string) (net.Conn, error)
}

// DisabledSafeHTTPClient is the fail-closed default until an application
// provides the production-safe network adapter.
type DisabledSafeHTTPClient struct{}

func (DisabledSafeHTTPClient) DoPublic(context.Context, *http.Request) (*http.Response, error) {
	return nil, ErrOutboundHTTPUnavailable
}

// PublicHTTPClient resolves every address before dialing and pins the
// connection to the validated address set. Redirects are returned to callers
// and never followed.
type PublicHTTPClient struct {
	Resolver *net.Resolver
	Timeout  time.Duration
	Dialer   ContextDialer
	RootCAs  *x509.CertPool
}

func (c PublicHTTPClient) DoPublic(ctx context.Context, request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.URL.User != nil || request.URL.Fragment != "" || (request.URL.Scheme != "http" && request.URL.Scheme != "https") {
		return nil, errors.New("outbound URL is not allowed")
	}
	if request.URL.Host == "" || strings.HasSuffix(request.URL.Host, ":") {
		return nil, errors.New("outbound URL authority is invalid")
	}
	host, err := normalizePublicHost(request.URL.Hostname())
	if err != nil {
		return nil, errors.New("outbound URL host is invalid")
	}
	port := request.URL.Port()
	if port == "" {
		if request.URL.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if port != "80" && port != "443" {
		return nil, errors.New("outbound URL port is not allowed")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, errors.New("outbound URL port is invalid")
	}
	addresses, err := c.resolvePublic(ctx, host)
	if err != nil {
		return nil, err
	}
	transport := c.publicTransport(host, port, addresses)
	timeout := c.Timeout
	if timeout <= 0 || timeout > 10*time.Second {
		timeout = 10 * time.Second
	}
	client := &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(request.WithContext(ctx))
	if err != nil {
		transport.CloseIdleConnections()
		return nil, err
	}
	response.Body = &limitedReadCloser{ReadCloser: response.Body, reader: io.LimitReader(response.Body, 4097), close: transport.CloseIdleConnections}
	return response, nil
}

func normalizePublicHost(host string) (string, error) {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" {
		return "", errors.New("outbound URL host is missing")
	}
	if address, err := netip.ParseAddr(host); err == nil {
		if address.Zone() != "" {
			return "", errors.New("outbound URL host zone is not allowed")
		}
		return address.Unmap().String(), nil
	}
	ascii, err := idna.Lookup.ToASCII(host)
	if err != nil || ascii == "" {
		return "", errors.New("outbound URL host is invalid")
	}
	return strings.ToLower(ascii), nil
}

func (c PublicHTTPClient) publicTransport(host, port string, addresses []netip.Addr) *http.Transport {
	expectedHost, hostErr := normalizePublicHost(host)
	expectedAuthority := net.JoinHostPort(expectedHost, port)
	validated := append([]netip.Addr(nil), addresses...)
	roots := c.RootCAs
	if roots != nil {
		roots = roots.Clone()
	}
	dialer := c.Dialer
	if dialer == nil {
		dialer = &net.Dialer{Timeout: 3 * time.Second, KeepAlive: 30 * time.Second}
	}
	return &http.Transport{
		Proxy: nil,
		TLSClientConfig: &tls.Config{
			ServerName: expectedHost,
			RootCAs:    roots,
		},
		DialContext: func(dialCtx context.Context, network, authority string) (net.Conn, error) {
			if hostErr != nil || (network != "tcp" && network != "tcp4" && network != "tcp6") {
				return nil, errors.New("outbound dial target is invalid")
			}
			dialHost, dialPort, err := net.SplitHostPort(authority)
			if err != nil {
				return nil, errors.New("outbound dial authority is invalid")
			}
			dialHost, err = normalizePublicHost(dialHost)
			if err != nil || dialHost != expectedHost || dialPort != port || net.JoinHostPort(dialHost, dialPort) != expectedAuthority {
				return nil, errors.New("outbound dial authority changed after validation")
			}
			if len(validated) == 0 {
				return nil, errors.New("outbound address set is empty")
			}
			for _, address := range validated {
				if !isPublicAddress(address) {
					return nil, errors.New("outbound address set contains a non-public address")
				}
			}
			var last error
			for _, address := range validated {
				conn, dialErr := dialer.DialContext(dialCtx, network, net.JoinHostPort(address.String(), port))
				if dialErr == nil {
					return conn, nil
				}
				last = dialErr
			}
			if last == nil {
				last = errors.New("outbound dial failed")
			}
			return nil, last
		},
		TLSHandshakeTimeout:   3 * time.Second,
		ResponseHeaderTimeout: 3 * time.Second,
		IdleConnTimeout:       30 * time.Second,
		MaxIdleConns:          4,
	}
}

func (c PublicHTTPClient) resolvePublic(ctx context.Context, host string) ([]netip.Addr, error) {
	if parsed, err := netip.ParseAddr(host); err == nil {
		parsed = parsed.Unmap()
		if !isPublicAddress(parsed) {
			return nil, errors.New("outbound host resolves to a non-public address")
		}
		return []netip.Addr{parsed}, nil
	}
	resolver := c.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	resolved, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil || len(resolved) == 0 {
		return nil, errors.New("outbound host DNS resolution failed")
	}
	addresses := make([]netip.Addr, 0, len(resolved))
	seen := make(map[netip.Addr]struct{}, len(resolved))
	for _, raw := range resolved {
		address := raw.Unmap()
		if !isPublicAddress(address) {
			return nil, errors.New("outbound host resolves to a non-public address")
		}
		if _, ok := seen[address]; !ok {
			seen[address] = struct{}{}
			addresses = append(addresses, address)
		}
	}
	return addresses, nil
}

// nonPublicPrefixes lists IANA special-purpose ranges that the standard
// library predicates do not reject (shared, documentation, benchmarking,
// reserved, translation and similar blocks).
var nonPublicPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.88.99.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("3fff::/20"),
	netip.MustParsePrefix("5f00::/16"),
}

func isPublicAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsMulticast() || address.IsUnspecified() {
		return false
	}
	for _, prefix := range nonPublicPrefixes {
		if prefix.Contains(address) {
			return false
		}
	}
	return true
}

type limitedReadCloser struct {
	io.ReadCloser
	reader io.Reader
	close  func()
	read   int
}

func (r *limitedReadCloser) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.read += n
	if r.read > 4096 {
		return n, errors.New("outbound response exceeds 4096 bytes")
	}
	return n, err
}
func (r *limitedReadCloser) Close() error {
	err := r.ReadCloser.Close()
	if r.close != nil {
		r.close()
	}
	return err
}
