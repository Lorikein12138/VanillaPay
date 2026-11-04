package testsupport

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
	"vanillapay/internal/platform"
)

const (
	callbackLogicalIP  = "93.184.216.34"
	turnstileLogicalIP = "93.184.216.35"
	secondaryLogicalIP = "93.184.216.36"
)

type RecordedHTTPRequest struct {
	Method string
	Host   string
	Path   string
}

type PublicNetworkEvidence struct {
	DNSQueries    []string
	LogicalDials  []string
	PhysicalDials []string
	ServerNames   []string
	HTTPRequests  []RecordedHTTPRequest
	Rejected      int
}

// PublicNetwork is an isolated DNS/TLS fixture. The production client sees
// public logical IPs, while the fixture's raw TCP dialer maps only those
// prevalidated targets to listeners bound on loopback.
type PublicNetwork struct {
	dnsTCP   net.Listener
	dnsUDP   net.PacketConn
	dnsAddr  string
	resolver *net.Resolver

	caA      *testCA
	caB      *testCA
	rootPool *x509.CertPool

	mu             sync.Mutex
	answers        map[string][]netip.Addr
	mappings       map[string]string
	localListeners map[string]struct{}
	dnsQueries     []string
	logicalDials   []string
	physicalDials  []string
	serverNames    []string
	httpRequests   []RecordedHTTPRequest
	rejected       int
	beforeDial     func()
	dnsCloseOnce   sync.Once
}

type testCA struct {
	certificate *x509.Certificate
	privateKey  *ecdsa.PrivateKey
}

func NewPublicNetwork(t *testing.T) *PublicNetwork {
	t.Helper()
	caA := newTestCA(t, "VanillaPay local test CA A")
	caB := newTestCA(t, "VanillaPay local test CA B")
	dnsTCP, dnsUDP, err := listenDNSPair()
	if err != nil {
		t.Fatalf("start matching local DNS TCP/UDP listeners: %v", err)
	}
	network := &PublicNetwork{
		dnsTCP:         dnsTCP,
		dnsUDP:         dnsUDP,
		dnsAddr:        dnsTCP.Addr().String(),
		caA:            caA,
		caB:            caB,
		rootPool:       x509.NewCertPool(),
		answers:        make(map[string][]netip.Addr),
		mappings:       make(map[string]string),
		localListeners: make(map[string]struct{}),
	}
	network.rootPool.AddCert(caA.certificate)
	network.resolver = &net.Resolver{
		PreferGo:     true,
		StrictErrors: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			dialNetwork := "udp4"
			address := dnsUDP.LocalAddr().String()
			if strings.HasPrefix(network, "tcp") {
				dialNetwork = "tcp4"
				address = dnsTCP.Addr().String()
			}
			return (&net.Dialer{}).DialContext(ctx, dialNetwork, address)
		},
	}
	go network.serveDNSUDP()
	go network.serveDNSTCP()
	t.Cleanup(network.closeDNS)
	return network
}

func listenDNSPair() (net.Listener, net.PacketConn, error) {
	const attempts = 64
	var lastErr error
	for range attempts {
		udp, err := net.ListenPacket("udp4", "127.0.0.1:0")
		if err != nil {
			lastErr = err
			continue
		}
		udpAddress, ok := udp.LocalAddr().(*net.UDPAddr)
		if !ok {
			_ = udp.Close()
			lastErr = fmt.Errorf("local DNS UDP listener has unexpected address type %T", udp.LocalAddr())
			continue
		}
		tcp, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(udpAddress.Port)))
		if err == nil {
			return tcp, udp, nil
		}
		_ = udp.Close()
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no DNS port pair was available")
	}
	return nil, nil, fmt.Errorf("unable to bind a shared TCP/UDP port after %d attempts: %w", attempts, lastErr)
}

func (n *PublicNetwork) SetDNS(host string, answers []netip.Addr) {
	normalized, err := normalizeFixtureHost(host)
	if err != nil {
		panic(err)
	}
	n.mu.Lock()
	n.answers[normalized] = append([]netip.Addr(nil), answers...)
	n.mu.Unlock()
}

func (n *PublicNetwork) Client(timeout time.Duration) platform.PublicHTTPClient {
	return platform.PublicHTTPClient{Resolver: n.resolver, Timeout: timeout, Dialer: n, RootCAs: n.RootCAs()}
}

func (n *PublicNetwork) RootCAs() *x509.CertPool { return n.rootPool.Clone() }

func (n *PublicNetwork) ServeTLS(t *testing.T, host string, handler http.Handler) string {
	t.Helper()
	normalized, err := normalizeFixtureHost(host)
	if err != nil {
		t.Fatal(err)
	}
	certificate := n.IssueCertificate(t, []string{normalized}, "A", time.Now().Add(-5*time.Minute), time.Now().Add(time.Hour))
	return n.ServeTLSWithCertificate(t, normalized, certificate, handler)
}

func (n *PublicNetwork) IssueCertificate(t *testing.T, names []string, issuer string, notBefore, notAfter time.Time) tls.Certificate {
	t.Helper()
	var ca *testCA
	switch issuer {
	case "A":
		ca = n.caA
	case "B":
		ca = n.caB
	default:
		t.Fatalf("unknown local certificate issuer %q", issuer)
	}
	if len(names) == 0 || !notAfter.After(notBefore) {
		t.Fatal("certificate needs identity names and a positive validity interval")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate local endpoint key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate local certificate serial: %v", err)
	}
	leaf := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: names[0]},
		NotBefore:    notBefore,
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, name := range names {
		if ip, parseErr := netip.ParseAddr(name); parseErr == nil {
			leaf.IPAddresses = append(leaf.IPAddresses, net.ParseIP(ip.String()))
		} else {
			leaf.DNSNames = append(leaf.DNSNames, name)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca.certificate, &key.PublicKey, ca.privateKey)
	if err != nil {
		t.Fatalf("sign local endpoint certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der, ca.certificate.Raw}, PrivateKey: key, Leaf: leaf}
}

func (n *PublicNetwork) ServeTLSWithCertificate(t *testing.T, host string, certificate tls.Certificate, handler http.Handler) string {
	t.Helper()
	normalized, err := normalizeFixtureHost(host)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start local TLS endpoint: %v", err)
	}
	localAddress := listener.Addr().String()
	n.mu.Lock()
	n.localListeners[localAddress] = struct{}{}
	answers := append([]netip.Addr(nil), n.answers[normalized]...)
	n.mu.Unlock()
	if len(answers) == 0 {
		_ = listener.Close()
		t.Fatalf("no local logical DNS answer is registered for %s", normalized)
	}
	for _, address := range answers {
		n.MapIP(t, netip.AddrPortFrom(address, 443), localAddress)
	}
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			n.mu.Lock()
			n.serverNames = append(n.serverNames, hello.ServerName)
			n.mu.Unlock()
			return nil, nil
		},
	}
	if handler == nil {
		handler = http.NotFoundHandler()
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.mu.Lock()
		n.httpRequests = append(n.httpRequests, RecordedHTTPRequest{Method: r.Method, Host: r.Host, Path: r.URL.Path})
		n.mu.Unlock()
		handler.ServeHTTP(w, r)
	})}
	go func() { _ = server.Serve(tls.NewListener(listener, tlsConfig)) }()
	t.Cleanup(func() {
		_ = server.Close()
		n.mu.Lock()
		delete(n.localListeners, localAddress)
		for key, mapped := range n.mappings {
			if mapped == localAddress {
				delete(n.mappings, key)
			}
		}
		n.mu.Unlock()
	})
	return "https://" + normalized
}

func (n *PublicNetwork) MapIP(t *testing.T, logical netip.AddrPort, localAddress string) {
	t.Helper()
	logical = netip.AddrPortFrom(logical.Addr().Unmap(), logical.Port())
	if !logical.IsValid() || !fixturePublicAddress(logical.Addr()) || (logical.Port() != 443 && logical.Port() != 465) {
		t.Fatalf("invalid public logical endpoint %s", logical)
	}
	local, err := netip.ParseAddrPort(localAddress)
	if err != nil || !local.Addr().IsLoopback() {
		t.Fatalf("physical endpoint must be a loopback listener: %q", localAddress)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.localListeners[localAddress]; !ok {
		t.Fatalf("physical endpoint is not owned by this fixture: %q", localAddress)
	}
	n.mappings[logical.String()] = localAddress
}

func (n *PublicNetwork) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	n.mu.Lock()
	physical, err := resolvePublicEndpoint(network, address, n.mappings)
	if err != nil {
		n.rejected++
		n.mu.Unlock()
		return nil, err
	}
	n.logicalDials = append(n.logicalDials, network+"|"+address)
	hook := n.beforeDial
	n.beforeDial = nil
	n.mu.Unlock()
	if hook != nil {
		hook()
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, network, physical)
	if err != nil {
		n.mu.Lock()
		n.rejected++
		n.mu.Unlock()
		return nil, err
	}
	n.mu.Lock()
	n.physicalDials = append(n.physicalDials, conn.RemoteAddr().String())
	n.mu.Unlock()
	return conn, nil
}

func (n *PublicNetwork) BeforeDial(hook func()) {
	n.mu.Lock()
	n.beforeDial = hook
	n.mu.Unlock()
}

func (n *PublicNetwork) Evidence() PublicNetworkEvidence {
	n.mu.Lock()
	defer n.mu.Unlock()
	return PublicNetworkEvidence{
		DNSQueries:    append([]string(nil), n.dnsQueries...),
		LogicalDials:  append([]string(nil), n.logicalDials...),
		PhysicalDials: append([]string(nil), n.physicalDials...),
		ServerNames:   append([]string(nil), n.serverNames...),
		HTTPRequests:  append([]RecordedHTTPRequest(nil), n.httpRequests...),
		Rejected:      n.rejected,
	}
}

func (n *PublicNetwork) TLSHTTPClient(host string) *http.Client {
	normalized, err := normalizeFixtureHost(host)
	if err != nil {
		panic(err)
	}
	transport := &http.Transport{
		Proxy:           nil,
		TLSClientConfig: &tls.Config{RootCAs: n.RootCAs()},
		DialContext: func(ctx context.Context, network, authority string) (net.Conn, error) {
			dialHost, port, splitErr := net.SplitHostPort(authority)
			if splitErr != nil || strings.ToLower(strings.TrimSuffix(dialHost, ".")) != normalized || port != "443" {
				n.mu.Lock()
				n.rejected++
				n.mu.Unlock()
				return nil, errors.New("local TLS client target is not registered")
			}
			n.mu.Lock()
			answers := append([]netip.Addr(nil), n.answers[normalized]...)
			n.mu.Unlock()
			if len(answers) != 1 {
				return nil, errors.New("local TLS client requires one registered address")
			}
			return n.DialContext(ctx, network, net.JoinHostPort(answers[0].String(), port))
		},
		TLSHandshakeTimeout: 3 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func resolvePublicEndpoint(network, address string, mappings map[string]string) (string, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return "", fmt.Errorf("unsupported outbound network %q", network)
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", errors.New("outbound logical address is invalid")
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || ip.Zone() != "" {
		return "", errors.New("outbound logical address must be a literal IP")
	}
	ip = ip.Unmap()
	if (network == "tcp4" && !ip.Is4()) || (network == "tcp6" && !ip.Is6()) {
		return "", errors.New("outbound logical address family does not match network")
	}
	logical := net.JoinHostPort(ip.String(), port)
	physical, ok := mappings[logical]
	if !ok {
		return "", errors.New("outbound logical address is not mapped")
	}
	local, err := netip.ParseAddrPort(physical)
	if err != nil || !local.Addr().IsLoopback() || local.Port() == 0 {
		return "", errors.New("outbound mapping is not a local listener")
	}
	return physical, nil
}

func (n *PublicNetwork) serveDNSUDP() {
	buffer := make([]byte, 4096)
	for {
		size, remote, err := n.dnsUDP.ReadFrom(buffer)
		if err != nil {
			return
		}
		response, packErr := n.answerDNS(buffer[:size])
		if packErr == nil {
			_, _ = n.dnsUDP.WriteTo(response, remote)
		}
	}
}

func (n *PublicNetwork) serveDNSTCP() {
	for {
		connection, err := n.dnsTCP.Accept()
		if err != nil {
			return
		}
		go n.handleDNSTCPConnection(connection)
	}
}

func (n *PublicNetwork) handleDNSTCPConnection(connection net.Conn) {
	defer func() { _ = connection.Close() }() // Cleanup must not replace the operation result.
	for {
		var length [2]byte
		if _, err := io.ReadFull(connection, length[:]); err != nil {
			return
		}
		query := make([]byte, binary.BigEndian.Uint16(length[:]))
		if _, err := io.ReadFull(connection, query); err != nil {
			return
		}
		response, err := n.answerDNS(query)
		if err != nil || len(response) > 65535 {
			return
		}
		binary.BigEndian.PutUint16(length[:], uint16(len(response)))
		if _, err := connection.Write(append(length[:], response...)); err != nil {
			return
		}
	}
}

func (n *PublicNetwork) answerDNS(query []byte) ([]byte, error) {
	var request dnsmessage.Message
	if err := request.Unpack(query); err != nil {
		return nil, err
	}
	response := dnsmessage.Message{Header: dnsmessage.Header{
		ID:                 request.ID,
		Response:           true,
		Authoritative:      true,
		RecursionDesired:   request.RecursionDesired,
		RecursionAvailable: true,
		RCode:              dnsmessage.RCodeSuccess,
	}, Questions: request.Questions}
	n.mu.Lock()
	defer n.mu.Unlock()
	for _, question := range request.Questions {
		host := strings.TrimSuffix(strings.ToLower(question.Name.String()), ".")
		n.dnsQueries = append(n.dnsQueries, fmt.Sprintf("%s/%v", host, question.Type))
		for _, answer := range n.answers[host] {
			switch question.Type {
			case dnsmessage.TypeA:
				if answer.Is4() {
					response.Answers = append(response.Answers, dnsmessage.Resource{Header: dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 1}, Body: &dnsmessage.AResource{A: answer.As4()}})
				}
			case dnsmessage.TypeAAAA:
				if answer.Is6() {
					response.Answers = append(response.Answers, dnsmessage.Resource{Header: dnsmessage.ResourceHeader{Name: question.Name, Type: dnsmessage.TypeAAAA, Class: dnsmessage.ClassINET, TTL: 1}, Body: &dnsmessage.AAAAResource{AAAA: answer.As16()}})
				}
			}
		}
	}
	return response.Pack()
}

func (n *PublicNetwork) closeDNS() {
	n.dnsCloseOnce.Do(func() {
		_ = n.dnsUDP.Close()
		_ = n.dnsTCP.Close()
	})
}

func newTestCA(t *testing.T, commonName string) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate local CA key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate local CA serial: %v", err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(2 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create local CA: %v", err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse local CA: %v", err)
	}
	return &testCA{certificate: certificate, privateKey: key}
}

func normalizeFixtureHost(host string) (string, error) {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "" || strings.ContainsAny(host, "/:@[]") {
		return "", fmt.Errorf("invalid fixture host %q", host)
	}
	return host, nil
}

func fixturePublicAddress(address netip.Addr) bool {
	return address.IsValid() && address.IsGlobalUnicast() && !address.IsPrivate() && !address.IsLoopback() && !address.IsLinkLocalUnicast() && !address.IsMulticast() && !address.IsUnspecified()
}
