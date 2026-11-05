package settings

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) { os.Exit(runWithSMTPTrust(m)) }

func runWithSMTPTrust(m *testing.M) int {
	// Install one process-lifetime root before any fixture can initialize x509's
	// cached system pool. Per-test Setenv is too late and its file is short-lived.
	certificates, err := localSMTPCertificate()
	if err != nil {
		fmt.Fprintln(os.Stderr, "create SMTP fixture certificates")
		return 1
	}
	dir, err := os.MkdirTemp("", "vanillapay-smtp-trust-")
	if err != nil {
		fmt.Fprintln(os.Stderr, "create SMTP fixture trust directory")
		return 1
	}
	defer func() { _ = os.RemoveAll(dir) }() // Remove the public CA file after every test has stopped.
	path := filepath.Join(dir, "root.pem")
	if err := os.WriteFile(path, certificates.rootPEM, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, "write SMTP fixture trust root")
		return 1
	}
	if err := os.Setenv("SSL_CERT_FILE", path); err != nil {
		fmt.Fprintln(os.Stderr, "configure SMTP fixture trust root")
		return 1
	}
	return m.Run()
}
