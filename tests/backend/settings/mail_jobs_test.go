package settings

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"vanillapay/internal/app"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
	internalsettings "vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_RefundOfflineAndAnnouncementMailJobs(t *testing.T) {
	db := testsupport.NewDatabase(t)
	app := db.NewApplication(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "mail job billing eligibility", 1000, 3650, []string{"alipay"}, nil)
	db.PrepareBillingMerchant(t, app, merchantID, merchantKey, planID)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}

	order, err := app.Services.Payment.Create(t.Context(), payment.CreateCommand{MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "mail-job-refund", Name: "mail job refund", OriginalCents: 800, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.181", Actor: actor})
	if err != nil {
		t.Fatalf("create refund mail fixture order: %v", err)
	}
	if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, order.TradeNo, payment.SupplementRequest{Reason: "mail job fixture", Channel: "alipay"}, "mail-job-paid"); err != nil {
		t.Fatalf("pay refund mail fixture order: %v", err)
	}
	refundDigest := sha256.Sum256([]byte("mail job refund request"))
	refund, err := app.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{MerchantID: merchantID, TradeNo: order.TradeNo, OutRefundNo: "mail-job-refund-001", AmountCents: 800, Reason: "customer request", RequestDigest: refundDigest[:], Actor: actor})
	if err != nil {
		t.Fatalf("create refund request: %v", err)
	}
	if _, err := app.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{MerchantID: merchantID, TradeNo: order.TradeNo, OutRefundNo: "mail-job-refund-001", AmountCents: 800, Reason: "customer request", Actor: actor}); err != nil {
		t.Fatalf("repeat identical refund request: %v", err)
	}

	if _, err := db.Pool.Exec(t.Context(), `UPDATE devices SET last_seen_at=$1 WHERE merchant_id=$2 AND revoked_at IS NULL`, db.Clock.Now().Add(-91*time.Second), merchantID); err != nil {
		t.Fatalf("make billing device offline: %v", err)
	}
	if count, err := app.Services.Device.DetectOffline(t.Context(), 10); err != nil || count != 1 {
		t.Fatalf("first offline episode count=%d err=%v", count, err)
	}
	if count, err := app.Services.Device.DetectOffline(t.Context(), 10); err != nil || count != 0 {
		t.Fatalf("repeated offline scan count=%d err=%v", count, err)
	}
	if _, err := app.Services.Portal.SaveAnnouncement(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, portal.AnnouncementCommand{
		Title: portal.LocalizedText{ZhCN: "公告不发邮件", En: "No announcement mail"},
		Body:  portal.LocalizedText{ZhCN: "公告正文", En: "Announcement body"}, Popup: true,
	}); err != nil {
		t.Fatalf("publish announcement fixture: %v", err)
	}

	t.Run("V39.B.r01/v01", func(t *testing.T) {
		var refundJobs, offlineJobs, announcementJobs int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM mail_deliveries WHERE dedupe_key LIKE 'refund:%'`).Scan(&refundJobs); err != nil {
			t.Fatal("count refund mail jobs")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM mail_deliveries WHERE dedupe_key LIKE 'device_offline:%'`).Scan(&offlineJobs); err != nil {
			t.Fatal("count offline episode mail jobs")
		}
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM mail_deliveries WHERE template LIKE '%announcement%'`).Scan(&announcementJobs); err != nil {
			t.Fatal("count announcement mail jobs")
		}
		var pendingRefunds int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM refunds WHERE public_id=$1 AND status='pending'`, refund.ID).Scan(&pendingRefunds); err != nil || pendingRefunds != 1 || refundJobs != 1 || offlineJobs != 1 || announcementJobs != 0 {
			t.Fatalf("durable mail job counts refund=%d offline=%d announcement=%d pending_refunds=%d err=%v", refundJobs, offlineJobs, announcementJobs, pendingRefunds, err)
		}
	})
}

func TestDB_MailWorkerDeliversThroughLocalTLS(t *testing.T) {
	db := testsupport.NewDatabase(t)
	app := db.NewApplication(t)
	sink := newTrustedLocalSMTPSink(t)
	configureMailWorker(t, db, app, "127.0.0.1", sink.port, "contract-smtp-user", fixturePassword)
	admin := platform.Actor{Role: "admin", SubjectID: 1}
	delivery, err := app.Services.Settings.QueueMailTest(t.Context(), admin, " Operator@Example.Test ")
	if err != nil {
		t.Fatalf("queue SMTP test mail: %v", err)
	}
	startMailWorkers(t, app)
	waitMailState(t, app, admin, delivery["delivery_id"].(string), "sent", 10*time.Second)
	select {
	case raw := <-sink.message:
		message, err := mail.ReadMessage(strings.NewReader(raw))
		if err != nil {
			t.Fatalf("parse locally received SMTP message: %v", err)
		}
		if got := strings.ToLower(message.Header.Get("To")); !strings.Contains(got, "operator@example.test") {
			t.Fatalf("SMTP recipient header=%q, want normalized merchant address", got)
		}
		decoder := new(mime.WordDecoder)
		subject, err := decoder.DecodeHeader(message.Header.Get("Subject"))
		if err != nil || subject != "VanillaPay SMTP test" {
			t.Fatalf("SMTP subject=%q err=%v", subject, err)
		}
		body, err := readMIMEBody(message)
		if err != nil || !strings.Contains(body, "SMTP 配置测试邮件。") {
			t.Fatalf("SMTP body did not preserve the configured Chinese test content: %q err=%v", body, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("local SMTP server did not receive the queued mail")
	}
	t.Run("V39.B.r02/v01", func(t *testing.T) {
		status, err := app.Services.Settings.MailTestStatus(t.Context(), admin, delivery["delivery_id"].(string))
		if err != nil || status["state"] != "sent" || mailStatusErrorCode(status) != "" {
			t.Fatalf("successful SMTP status=%#v err=%v", status, err)
		}
	})
	t.Run("V39.B.r03/v01", func(t *testing.T) {
		if sink.authenticatedUser != "contract-smtp-user" || sink.authenticatedPassword != fixturePassword {
			t.Fatal("local SMTP endpoint did not receive configured authentication")
		}
	})
}

func TestDB_MailWorkerRetriesFailuresWithoutRollingBackFacts(t *testing.T) {
	db := testsupport.NewDatabase(t)
	app := db.NewApplication(t)
	merchantID, merchantKey := db.SeedMerchant(t)
	planID := db.SeedPlan(t, "mail failure billing eligibility", 1000, 3650, []string{"alipay"}, nil)
	db.PrepareBillingMerchant(t, app, merchantID, merchantKey, planID)
	admin := platform.Actor{Role: "admin", SubjectID: 1}
	configureMailWorker(t, db, app, "127.0.0.1", unusedLocalPort(t), "secret-smtp-user", fixturePassword)
	delivery, err := app.Services.Settings.QueueMailTest(t.Context(), admin, "secret-recipient@example.test")
	if err != nil {
		t.Fatalf("queue failing mail test: %v", err)
	}

	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	order, err := app.Services.Payment.Create(t.Context(), payment.CreateCommand{MerchantID: merchantID, Protocol: "v1", Purpose: "external", OutTradeNo: "mail-fail-refund", Name: "mail failure refund", OriginalCents: 900, NotifyURL: "https://merchant.example.test/notify", SourceIP: "192.0.2.182", Actor: actor})
	if err != nil {
		t.Fatalf("create mail failure refund order: %v", err)
	}
	if err := app.Services.Payment.SupplementIdempotent(t.Context(), actor, order.TradeNo, payment.SupplementRequest{Reason: "mail failure fixture", Channel: "alipay"}, "mail-fail-paid"); err != nil {
		t.Fatalf("pay mail failure refund order: %v", err)
	}
	refundDigest := sha256.Sum256([]byte("mail failure refund request"))
	refund, err := app.Services.Payment.RequestRefund(t.Context(), payment.RefundCommand{MerchantID: merchantID, TradeNo: order.TradeNo, OutRefundNo: "mail-fail-refund-001", AmountCents: 900, Reason: "failure durability", RequestDigest: refundDigest[:], Actor: actor})
	if err != nil {
		t.Fatalf("persist refund before SMTP failure: %v", err)
	}
	if _, err := db.Pool.Exec(t.Context(), `UPDATE devices SET last_seen_at=$1 WHERE merchant_id=$2 AND revoked_at IS NULL`, db.Clock.Now().Add(-91*time.Second), merchantID); err != nil {
		t.Fatalf("prepare durable offline event: %v", err)
	}
	if count, err := app.Services.Device.DetectOffline(t.Context(), 10); err != nil || count != 1 {
		t.Fatalf("persist offline fact before SMTP failure count=%d err=%v", count, err)
	}
	startMailWorkers(t, app)
	waitMailAttempts(t, db, delivery["delivery_id"].(string), 1, 10*time.Second)
	assertMailAttempts(t, db, delivery["delivery_id"].(string), 1)

	t.Run("V39.B.r02/v02", func(t *testing.T) {
		currentAttempt := 1
		for _, delay := range []time.Duration{time.Minute, 5 * time.Minute, 30 * time.Minute} {
			notDue := fmt.Sprintf("%d seconds", int(delay/time.Second)-10)
			if _, err := db.Pool.Exec(t.Context(), `UPDATE mail_deliveries SET updated_at=now()-$1::interval WHERE public_id=$2`, notDue, delivery["delivery_id"].(string)); err != nil {
				t.Fatalf("set mail retry timestamp just before %s: %v", delay, err)
			}
			before := completedMailScanCount(t, db)
			enqueueMailScan(t, db)
			waitCompletedMailScan(t, db, before, 10*time.Second)
			assertMailAttempts(t, db, delivery["delivery_id"].(string), currentAttempt)

			due := fmt.Sprintf("%d seconds", int(delay/time.Second)+1)
			if _, err := db.Pool.Exec(t.Context(), `UPDATE mail_deliveries SET updated_at=now()-$1::interval WHERE public_id=$2`, due, delivery["delivery_id"].(string)); err != nil {
				t.Fatalf("set mail retry timestamp after %s: %v", delay, err)
			}
			before = completedMailScanCount(t, db)
			enqueueMailScan(t, db)
			waitCompletedMailScan(t, db, before, 10*time.Second)
			currentAttempt++
			waitMailAttempts(t, db, delivery["delivery_id"].(string), currentAttempt, 10*time.Second)
		}
		var attempts int
		var state, lastError string
		if err := db.Pool.QueryRow(t.Context(), `SELECT attempts,state,last_error FROM mail_deliveries WHERE public_id=$1`, delivery["delivery_id"]).Scan(&attempts, &state, &lastError); err != nil || attempts != 4 || state != "failed" || lastError != "smtp_delivery_failed" {
			t.Fatalf("final failed mail state attempts=%d state=%q last_error=%q err=%v", attempts, state, lastError, err)
		}
		status, err := app.Services.Settings.MailTestStatus(t.Context(), admin, delivery["delivery_id"].(string))
		if err != nil || status["state"] != "failed" || mailStatusErrorCode(status) != "smtp_delivery_failed" {
			t.Fatalf("mail status API did not expose final error code: %#v err=%v", status, err)
		}
	})
	t.Run("V39.B.r03/v02", func(t *testing.T) {
		status, err := app.Services.Settings.MailTestStatus(t.Context(), admin, delivery["delivery_id"].(string))
		encoded, marshalErr := json.Marshal(status)
		if err != nil || marshalErr != nil || strings.Contains(string(encoded), fixturePassword) || strings.Contains(string(encoded), "secret-recipient") || strings.Contains(string(encoded), "contract-smtp") {
			t.Fatalf("mail status exposed credential or recipient: %s err=%v marshal=%v", encoded, err, marshalErr)
		}
		var secretCipher, payloadCipher []byte
		if err := db.Pool.QueryRow(t.Context(), `SELECT recipient_cipher,payload_cipher FROM mail_deliveries WHERE public_id=$1`, delivery["delivery_id"]).Scan(&secretCipher, &payloadCipher); err != nil || strings.Contains(string(secretCipher), "secret-recipient") || strings.Contains(string(payloadCipher), fixturePassword) {
			t.Fatalf("mail delivery persisted a plaintext recipient or SMTP credential: err=%v", err)
		}
	})
	t.Run("V39.B.r04/v01", func(t *testing.T) {
		waitBusinessMailFailures(t, db, 2, 10*time.Second)
		var refundState string
		if err := db.Pool.QueryRow(t.Context(), `SELECT status FROM refunds WHERE public_id=$1`, refund.ID).Scan(&refundState); err != nil || refundState != "pending" {
			t.Fatalf("SMTP failure rolled back durable refund request: state=%q err=%v", refundState, err)
		}
		var offlineEpisode int
		if err := db.Pool.QueryRow(t.Context(), `SELECT offline_notified_episode FROM devices WHERE merchant_id=$1 AND revoked_at IS NULL`, merchantID).Scan(&offlineEpisode); err != nil || offlineEpisode != 1 {
			t.Fatalf("SMTP failure rolled back offline episode: episode=%d err=%v", offlineEpisode, err)
		}
		var queuedFacts int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM mail_deliveries WHERE state='failed' AND (dedupe_key LIKE 'refund:%' OR dedupe_key LIKE 'device_offline:%')`).Scan(&queuedFacts); err != nil || queuedFacts != 2 {
			t.Fatalf("SMTP failures removed persisted business mail rows: failed_fact_mail=%d err=%v", queuedFacts, err)
		}
	})
}

func mailStatusErrorCode(status map[string]any) string {
	value, ok := status["error_code"].(*string)
	if !ok || value == nil {
		return ""
	}
	return *value
}

func waitBusinessMailFailures(t *testing.T, db *testsupport.Database, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM mail_deliveries WHERE state='failed' AND (dedupe_key LIKE 'refund:%' OR dedupe_key LIKE 'device_offline:%')`).Scan(&count); err != nil {
			t.Fatalf("count failed durable business mail jobs: %v", err)
		}
		if count >= want {
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatal("context canceled while waiting for business mail failure records")
		case <-time.After(25 * time.Millisecond):
		}
	}
	t.Fatalf("durable business mail failures did not reach %d", want)
}

func configureMailWorker(t *testing.T, db *testsupport.Database, application *app.App, host string, port int, username, password string) {
	t.Helper()
	var version int64
	if err := db.Pool.QueryRow(t.Context(), `SELECT version FROM site_settings WHERE singleton=true`).Scan(&version); err != nil {
		t.Fatalf("read mail settings version: %v", err)
	}
	value, err := json.Marshal(map[string]any{"host": host, "port": port, "username": username, "password": password, "from_address": "noreply@example.test", "from_name": "Contract Mail", "tls_mode": "tls"})
	if err != nil {
		t.Fatalf("encode local SMTP settings: %v", err)
	}
	_, err = application.Services.Settings.UpdateGroup(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, "mail", internalsettings.UpdateGroup{ExpectedVersion: version, Value: value})
	if err != nil {
		t.Fatalf("configure real local SMTP endpoint: %v", err)
	}
}

func startMailWorkers(t *testing.T, application *app.App) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	if err := application.Services.Jobs.Start(ctx); err != nil {
		cancel()
		t.Fatalf("start production River workers: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		if err := application.Services.Jobs.Stop(stopCtx); err != nil {
			t.Errorf("stop production River workers: %v", err)
		}
		cancel()
	})
}

func waitMailState(t *testing.T, application *app.App, actor platform.Actor, id, state string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := application.Services.Settings.MailTestStatus(t.Context(), actor, id)
		if err == nil && status["state"] == state {
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatal("context canceled while waiting for mail delivery")
		case <-time.After(25 * time.Millisecond):
		}
	}
	status, err := application.Services.Settings.MailTestStatus(t.Context(), actor, id)
	t.Fatalf("mail delivery did not reach state %q: status=%#v err=%v", state, status, err)
}

func waitMailAttempts(t *testing.T, db *testsupport.Database, id string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var attempts int
		var state string
		if err := db.Pool.QueryRow(t.Context(), `SELECT attempts,state FROM mail_deliveries WHERE public_id=$1`, id).Scan(&attempts, &state); err != nil {
			t.Fatalf("read mail attempts: %v", err)
		}
		if attempts >= want && state == "failed" {
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatal("context canceled while waiting for mail retry")
		case <-time.After(25 * time.Millisecond):
		}
	}
	var attempts int
	var state string
	_ = db.Pool.QueryRow(t.Context(), `SELECT attempts,state FROM mail_deliveries WHERE public_id=$1`, id).Scan(&attempts, &state)
	t.Fatalf("mail attempts=%d state=%q, want at least %d completed failure(s)", attempts, state, want)
}

func assertMailAttempts(t *testing.T, db *testsupport.Database, id string, want int) {
	t.Helper()
	var attempts int
	if err := db.Pool.QueryRow(t.Context(), `SELECT attempts FROM mail_deliveries WHERE public_id=$1`, id).Scan(&attempts); err != nil || attempts != want {
		t.Fatalf("mail attempts=%d, want %d; err=%v", attempts, want, err)
	}
}

func completedMailScanCount(t *testing.T, db *testsupport.Database) int {
	t.Helper()
	var count int
	if err := db.Pool.QueryRow(t.Context(), `SELECT COALESCE(sum(attempt),0) FROM river_job WHERE kind='vanillapay_mail_scan' AND state='completed'`).Scan(&count); err != nil {
		t.Fatalf("count completed mail scan jobs: %v", err)
	}
	return count
}

func enqueueMailScan(t *testing.T, db *testsupport.Database) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		tag, err := db.Pool.Exec(t.Context(), `UPDATE river_job SET state='available',finalized_at=NULL,scheduled_at=now() WHERE id=(SELECT id FROM river_job WHERE kind='vanillapay_mail_scan' AND state='completed' ORDER BY created_at DESC LIMIT 1)`)
		if err != nil {
			t.Fatalf("redeliver completed production mail scan job: %v", err)
		}
		if tag.RowsAffected() == 1 {
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatal("context canceled while waiting for a completed mail scan job")
		case <-time.After(25 * time.Millisecond):
		}
	}
	t.Fatal("production mail scan job did not complete before redelivery")
}

func waitCompletedMailScan(t *testing.T, db *testsupport.Database, before int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if completedMailScanCount(t, db) > before {
			return
		}
		select {
		case <-t.Context().Done():
			t.Fatal("context canceled while waiting for mail scan job")
		case <-time.After(25 * time.Millisecond):
		}
	}
	t.Fatalf("completed mail scan count did not exceed %d", before)
}

type localSMTPSink struct {
	port                  int
	message               chan string
	authenticatedUser     string
	authenticatedPassword string
}

// Go caches system roots after the first TLS verification. All local SMTP
// servers in this process therefore share one ephemeral trust root.
type smtpCertificates struct {
	server  tls.Certificate
	rootPEM []byte
}

var localSMTPCertificate = sync.OnceValues(func() (smtpCertificates, error) {
	now := time.Now()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return smtpCertificates{}, err
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(now.UnixNano()), Subject: pkix.Name{CommonName: "VanillaPay local SMTP test root"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, SubjectKeyId: []byte{1, 2, 3, 4}}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return smtpCertificates{}, err
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		return smtpCertificates{}, err
	}
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return smtpCertificates{}, err
	}
	leafTemplate := &x509.Certificate{SerialNumber: big.NewInt(now.UnixNano() + 1), Subject: pkix.Name{CommonName: "127.0.0.1"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(12 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, ca, &leafKey.PublicKey, caKey)
	if err != nil {
		return smtpCertificates{}, err
	}
	leaf, err := x509.ParseCertificate(leafDER)
	if err != nil {
		return smtpCertificates{}, err
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)})
	serverCertificate, err := tls.X509KeyPair(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw}), keyPEM)
	return smtpCertificates{server: serverCertificate, rootPEM: certPEM}, err
})

func newTrustedLocalSMTPSink(t *testing.T) *localSMTPSink {
	t.Helper()
	certificates, err := localSMTPCertificate()
	if err != nil {
		t.Fatalf("create local SMTP certificates: %v", err)
	}
	listener, err := net.Listen("tcp", netip.AddrPortFrom(netip.MustParseAddr("127.0.0.1"), 0).String())
	if err != nil {
		t.Fatalf("listen for local SMTP TLS: %v", err)
	}
	tlsListener := tls.NewListener(listener, &tls.Config{Certificates: []tls.Certificate{certificates.server}, MinVersion: tls.VersionTLS12})
	sink := &localSMTPSink{port: listener.Addr().(*net.TCPAddr).Port, message: make(chan string, 1)}
	t.Cleanup(func() { _ = tlsListener.Close() })
	go sink.serve(tlsListener)
	return sink
}

func (sink *localSMTPSink) serve(listener net.Listener) {
	connection, err := listener.Accept()
	if err != nil {
		return
	}
	defer func() { _ = connection.Close() }() // Cleanup must not replace the operation result.
	reader := bufio.NewReader(connection)
	writer := bufio.NewWriter(connection)
	write := func(line string) bool {
		if _, err := fmt.Fprintf(writer, "%s\r\n", line); err != nil {
			return false
		}
		return writer.Flush() == nil
	}
	if !write("220 smtp-test ESMTP") {
		return
	}
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		command := strings.TrimSpace(line)
		upper := strings.ToUpper(command)
		switch {
		case strings.HasPrefix(upper, "EHLO ") || strings.HasPrefix(upper, "HELO "):
			if _, err := writer.WriteString("250-smtp-test\r\n250 AUTH PLAIN\r\n"); err != nil || writer.Flush() != nil {
				return
			}
		case strings.HasPrefix(upper, "AUTH PLAIN "):
			parts := strings.Fields(command)
			if len(parts) < 3 {
				_ = write("501 invalid auth")
				continue
			}
			raw, err := base64.StdEncoding.DecodeString(parts[2])
			fields := strings.Split(string(raw), "\x00")
			if err != nil || len(fields) != 3 {
				_ = write("535 authentication failed")
				continue
			}
			sink.authenticatedUser, sink.authenticatedPassword = fields[1], fields[2]
			if !write("235 authenticated") {
				return
			}
		case strings.HasPrefix(upper, "MAIL FROM:") || strings.HasPrefix(upper, "RCPT TO:"):
			if !write("250 accepted") {
				return
			}
		case upper == "DATA":
			if !write("354 end with dot") {
				return
			}
			var body strings.Builder
			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				dataLine = strings.TrimSuffix(strings.TrimSuffix(dataLine, "\n"), "\r")
				if dataLine == "." {
					break
				}
				if strings.HasPrefix(dataLine, "..") {
					dataLine = dataLine[1:]
				}
				body.WriteString(dataLine)
				body.WriteString("\r\n")
			}
			sink.message <- body.String()
			if !write("250 queued") {
				return
			}
		case upper == "QUIT":
			_ = write("221 bye")
			return
		default:
			if !write("250 accepted") {
				return
			}
		}
	}
}

func readMIMEBody(message *mail.Message) (string, error) {
	body := message.Body
	var reader = body
	switch strings.ToLower(message.Header.Get("Content-Transfer-Encoding")) {
	case "quoted-printable":
		reader = quotedprintable.NewReader(body)
	case "base64":
		reader = base64.NewDecoder(base64.StdEncoding, body)
	}
	decoded, err := io.ReadAll(reader)
	return string(decoded), err
}

func unusedLocalPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve local SMTP failure port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release local SMTP failure port: %v", err)
	}
	return port
}
