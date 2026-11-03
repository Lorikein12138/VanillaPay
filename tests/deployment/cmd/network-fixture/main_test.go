package main

import (
	"crypto/x509"
	"encoding/pem"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUnit_FixtureCertificateIdentities(t *testing.T) {
	root := &x509.Certificate{Raw: []byte("local test root certificate")}
	rootBundle := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.Raw})
	outDir := t.TempDir()
	manifest, err := writeFixtureMaterials(outDir, rootBundle)
	if err != nil {
		t.Fatalf("write isolated fixture materials: %v", err)
	}
	wantSANs := map[string][]string{
		"callback":  {callbackHost},
		"turnstile": {turnstileHost},
		"smtp":      {smtpHost},
		"nginx":     {"localhost", "127.0.0.1"},
	}
	if manifest.CAFingerprint == "" || len(manifest.Certificates) != len(wantSANs) {
		t.Fatalf("fixture identity manifest is incomplete: %#v", manifest)
	}
	for name, want := range wantSANs {
		got := manifest.SANs[name]
		if len(got) != len(want) {
			t.Fatalf("%s SANs = %#v, want %#v", name, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s SANs = %#v, want %#v", name, got, want)
			}
		}
		certPath := filepath.Join(outDir, name+"-cert.pem")
		encoded, err := os.ReadFile(certPath)
		if err != nil {
			t.Fatalf("read %s certificate: %v", name, err)
		}
		block, _ := pem.Decode(encoded)
		if block == nil {
			t.Fatalf("%s certificate is not PEM", name)
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("parse %s certificate: %v", name, err)
		}
		if cert.IsCA || len(cert.ExtKeyUsage) != 1 || cert.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
			t.Fatalf("%s certificate must be a leaf server certificate", name)
		}
		for _, san := range want {
			if ip := netParseIP(san); ip != nil {
				if !containsIP(cert.IPAddresses, ip) {
					t.Fatalf("%s certificate is missing IP SAN %s", name, san)
				}
			} else if !containsString(cert.DNSNames, san) {
				t.Fatalf("%s certificate is missing DNS SAN %s", name, san)
			}
		}
		keyPath := filepath.Join(outDir, name+"-key.pem")
		info, err := os.Stat(keyPath)
		if err != nil {
			t.Fatalf("stat %s private key: %v", name, err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
			t.Fatalf("%s private key permissions = %04o, want 0600", name, info.Mode().Perm())
		}
		keyBytes, err := os.ReadFile(keyPath)
		if err != nil {
			t.Fatalf("read %s private key: %v", name, err)
		}
		if strings.Contains(string(encoded), string(keyBytes)) {
			t.Fatalf("%s certificate file contains private key material", name)
		}
	}
	manifestBytes, err := os.ReadFile(filepath.Join(outDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manifestBytes), "PRIVATE KEY") || strings.Contains(string(manifestBytes), "BEGIN") {
		t.Fatal("fixture manifest leaked private key material")
	}
}

func TestUnit_FixtureScenarioRedaction(t *testing.T) {
	const secret = "fixture-token-should-not-be-recorded"
	got := safeEndpointRecord("POST", "https://callback.vp.test/notify?token="+secret, []byte("password="+secret))
	if got != "POST /notify" || strings.Contains(got, secret) {
		t.Fatalf("safe endpoint evidence = %q", got)
	}
	if err := validateFixtureText(secret+"\nleak", 128); err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("invalid fixture text must fail without echoing sensitive input: %v", err)
	}
	if err := validateFixtureText("register", 64); err != nil {
		t.Fatalf("valid non-sensitive scenario rejected: %v", err)
	}
}

func netParseIP(value string) net.IP {
	return net.ParseIP(value)
}

func containsIP(values []net.IP, expected net.IP) bool {
	for _, value := range values {
		if value.Equal(expected) {
			return true
		}
	}
	return false
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
