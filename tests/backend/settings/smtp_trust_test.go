package settings

import (
	"crypto/tls"
	"crypto/x509"
	"net"
	"strconv"
	"testing"
)

func TestSMTPTrustSurvivesEarlierRootPoolInitialization(t *testing.T) {
	// Another TLS fixture may initialize Go's process-wide root cache first.
	if _, err := x509.SystemCertPool(); err != nil {
		t.Fatal("initialize system roots before creating the SMTP fixture")
	}
	sink := newTrustedLocalSMTPSink(t)
	connection, err := tls.Dial("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(sink.port)), &tls.Config{ServerName: "127.0.0.1", MinVersion: tls.VersionTLS12})
	if err != nil {
		t.Fatal("SMTP test root was not installed before the first TLS verification")
	}
	if err := connection.Close(); err != nil {
		t.Fatal("close SMTP trust probe")
	}
}
