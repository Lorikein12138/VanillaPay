package platform

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
)

type recordedDialer struct {
	addresses []string
	err       error
}

func (d *recordedDialer) DialContext(_ context.Context, network, address string) (net.Conn, error) {
	d.addresses = append(d.addresses, network+"|"+address)
	if d.err != nil {
		return nil, d.err
	}
	client, server := net.Pipe()
	_ = server.Close()
	return client, nil
}

func TestUnit_PublicHTTPClientRejectsUnsafeTargetsBeforeDial(t *testing.T) {
	dialer := &recordedDialer{}
	var dnsLookups atomic.Int32
	resolver := &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		dnsLookups.Add(1)
		return nil, errors.New("unexpected DNS lookup in unit test")
	}}
	client := PublicHTTPClient{Dialer: dialer, Resolver: resolver}
	for _, target := range []string{
		"http://127.0.0.1/callback",
		"http://10.0.0.1/callback",
		"http://[::1]/callback",
		"file://example.test/callback",
		"https://user@example.test/callback",
		"https://example.test:8443/callback",
		"https://example.test:/callback",
	} {
		t.Run(target, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, target, nil)
			if err != nil {
				t.Fatal(err)
			}
			if target == "https://example.test:/callback" {
				// url.Parse normalizes an empty explicit port on some Go versions;
				// preserve the raw authority form passed to the production port.
				request.URL.Host = "example.test:"
			}
			if _, err := client.DoPublic(context.Background(), request); err == nil {
				t.Fatalf("unsafe target was accepted: %s", target)
			}
		})
	}
	if len(dialer.addresses) != 0 {
		t.Fatalf("unsafe targets reached dialer: %v", dialer.addresses)
	}
	if got := dnsLookups.Load(); got != 0 {
		t.Fatalf("unsafe targets reached DNS resolver %d times", got)
	}

	transport := client.publicTransport("callback.vp.test", "443", []netip.Addr{netip.MustParseAddr("93.184.216.34"), netip.MustParseAddr("10.0.0.1")})
	defer transport.CloseIdleConnections()
	if _, err := transport.DialContext(context.Background(), "tcp", "callback.vp.test:443"); err == nil {
		t.Fatal("mixed public/private address set was accepted")
	}
	if len(dialer.addresses) != 0 {
		t.Fatalf("mixed address set reached dialer: %v", dialer.addresses)
	}
}

func TestUnit_PublicHTTPClientPinsValidatedIPs(t *testing.T) {
	dialer := &recordedDialer{err: errors.New("controlled dial result")}
	transport := (PublicHTTPClient{Dialer: dialer}).publicTransport("Callback.VP.Test.", "443", []netip.Addr{
		netip.MustParseAddr("93.184.216.34"),
		netip.MustParseAddr("93.184.216.35"),
	})
	defer transport.CloseIdleConnections()
	if _, err := transport.DialContext(context.Background(), "tcp", "callback.vp.test:443"); err == nil {
		t.Fatal("controlled dial error was lost")
	}
	want := []string{"tcp|93.184.216.34:443", "tcp|93.184.216.35:443"}
	if strings.Join(dialer.addresses, ",") != strings.Join(want, ",") {
		t.Fatalf("dial targets = %v, want validated IPs %v", dialer.addresses, want)
	}
	if transport.Proxy != nil {
		t.Fatal("outbound transport must not use a proxy")
	}
	if transport.TLSClientConfig == nil || transport.TLSClientConfig.ServerName != "callback.vp.test" || transport.TLSClientConfig.InsecureSkipVerify {
		t.Fatalf("TLS identity configuration = %#v", transport.TLSClientConfig)
	}
}

func TestUnit_PublicHTTPClientRejectsAuthorityChange(t *testing.T) {
	dialer := &recordedDialer{}
	transport := (PublicHTTPClient{Dialer: dialer}).publicTransport("callback.vp.test", "443", []netip.Addr{netip.MustParseAddr("93.184.216.34")})
	defer transport.CloseIdleConnections()
	for _, authority := range []string{"other.vp.test:443", "callback.vp.test:80", "93.184.216.34:443", "callback.vp.test:443:extra"} {
		if _, err := transport.DialContext(context.Background(), "tcp", authority); err == nil {
			t.Errorf("changed authority %q was accepted", authority)
		}
	}
	if len(dialer.addresses) != 0 {
		t.Fatalf("changed authority reached dialer: %v", dialer.addresses)
	}
}

func TestUnit_PublicHTTPClientBodyLimit(t *testing.T) {
	for _, size := range []int{4096, 4097} {
		t.Run(map[int]string{4096: "bytes_4096", 4097: "bytes_4097"}[size], func(t *testing.T) {
			body := &limitedReadCloser{ReadCloser: io.NopCloser(bytes.NewReader(bytes.Repeat([]byte{'x'}, size))), reader: io.LimitReader(bytes.NewReader(bytes.Repeat([]byte{'x'}, size)), 4097)}
			read, err := io.ReadAll(body)
			if size == 4096 && (err != nil || len(read) != 4096) {
				t.Fatalf("4096-byte body read = %d, %v", len(read), err)
			}
			if size == 4097 && (err == nil || !strings.Contains(err.Error(), "exceeds 4096")) {
				t.Fatalf("4097-byte body error = %v", err)
			}
		})
	}
}
