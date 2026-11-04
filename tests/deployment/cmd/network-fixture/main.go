package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
	"image"
	"image/color"
	"image/png"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	callbackAddress  = "93.184.216.34:443"
	turnstileAddress = "93.184.216.35:443"
	smtpAddress      = "93.184.216.36:465"
	callbackHost     = "callback.vp.test"
	turnstileHost    = "challenges.cloudflare.com"
	smtpHost         = "smtp.vp.test"
)

type fixtureManifest struct {
	Version        int                 `json:"version"`
	CreatedAt      time.Time           `json:"created_at"`
	CAFingerprint  string              `json:"ca_fingerprint_sha256"`
	Certificates   map[string]string   `json:"certificate_fingerprints_sha256"`
	SANs           map[string][]string `json:"sans"`
	MaterialFiles  []string            `json:"material_files"`
	RootBundleHash string              `json:"root_bundle_sha256"`
}

type fixtureState struct {
	Healthy               bool     `json:"healthy"`
	CallbackHTTP          int64    `json:"callback_http_requests"`
	CallbackCrashRequests int64    `json:"callback_crash_recovery_requests"`
	Siteverify            int64    `json:"siteverify_requests"`
	SMTPAccepted          int64    `json:"smtp_accepted_messages"`
	SMTPRejected          int64    `json:"smtp_rejected_messages"`
	ServerNames           []string `json:"tls_server_names"`
	UpdatedAt             string   `json:"updated_at"`
}

type stateStore struct {
	path          string
	emailCodePath string
	mu            sync.Mutex
	data          fixtureState
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: network-fixture prepare|serve|status|probe")
	}
	switch args[0] {
	case "prepare":
		flags := flag.NewFlagSet("prepare", flag.ContinueOnError)
		outDir := flags.String("output-dir", "/fixtures", "private output directory")
		rootBundle := flags.String("root-bundle", "/etc/ssl/certs/ca-certificates.crt", "existing system CA bundle")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		manifest, err := prepareMaterials(*outDir, *rootBundle)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(manifest)
	case "qr":
		flags := flag.NewFlagSet("qr", flag.ContinueOnError)
		payload := flags.String("payload", "", "payment QR content")
		output := flags.String("output", "", "private PNG output path")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		return writeQRPNG(*output, *payload)
	case "serve":
		flags := flag.NewFlagSet("serve", flag.ContinueOnError)
		stateFile := flags.String("state-file", "/fixtures/state.json", "non-sensitive state file")
		stateDir := flags.String("state-dir", "/fixtures", "certificate directory")
		emailCodeFile := flags.String("email-code-file", "/fixtures/last-email-code", "private test-only verification code capture")
		action := flags.String("siteverify-action", "register", "controlled Siteverify action")
		hostname := flags.String("siteverify-hostname", "localhost", "controlled Siteverify hostname")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if err := validateFixtureText(*action, 64); err != nil {
			return errors.New("invalid Siteverify action")
		}
		if err := validateFixtureText(*hostname, 253); err != nil {
			return errors.New("invalid Siteverify hostname")
		}
		return serve(*stateDir, *stateFile, *emailCodeFile, *action, *hostname)
	case "status":
		flags := flag.NewFlagSet("status", flag.ContinueOnError)
		stateFile := flags.String("state-file", "/fixtures/state.json", "non-sensitive state file")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		data, err := os.ReadFile(*stateFile)
		if err != nil {
			return errors.New("network fixture status is unavailable")
		}
		var state fixtureState
		if err := json.Unmarshal(data, &state); err != nil || !state.Healthy {
			return errors.New("network fixture is unhealthy")
		}
		return json.NewEncoder(os.Stdout).Encode(state)
	case "probe":
		return runOwnerProbe(args[1:])
	default:
		return fmt.Errorf("unknown network fixture command %q", args[0])
	}
}

// runOwnerProbe sends a real protocol request from the shared network namespace
// to the app's loopback listener. It emits only non-sensitive response evidence.
func runOwnerProbe(args []string) error {
	flags := flag.NewFlagSet("probe", flag.ContinueOnError)
	bodyName := flags.String("body-file", "", "form body file in the private fixture volume")
	requestID := flags.String("request-id", "", "unique request correlation identifier")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if filepath.Base(*bodyName) != *bodyName || *bodyName == "" || !regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`).MatchString(*bodyName) {
		return errors.New("invalid owner probe body file")
	}
	if err := validateFixtureText(*requestID, 128); err != nil || !regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`).MatchString(*requestID) {
		return errors.New("invalid owner probe request id")
	}
	bodyPath := filepath.Join("/run/fixtures", *bodyName)
	body, err := os.ReadFile(bodyPath)
	if err != nil {
		return errors.New("owner probe request body is unavailable")
	}
	defer func() { _ = os.Remove(bodyPath) }()
	var localAddress string
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if network != "tcp" && network != "tcp4" || address != "127.0.0.1:8080" {
				return nil, errors.New("owner probe destination is not the local application")
			}
			connection, dialErr := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp4", "127.0.0.1:8080")
			if dialErr == nil {
				localAddress = connection.LocalAddr().String()
			}
			return connection, dialErr
		},
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:8080/mapi.php", bytes.NewReader(body))
	if err != nil {
		return errors.New("owner probe request could not be constructed")
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("X-Request-ID", *requestID)
	request.Header.Set("X-Forwarded-For", "203.0.113.77")
	request.Header.Set("X-Real-IP", "203.0.113.78")
	request.Header.Set("Forwarded", "for=203.0.113.79")
	response, err := client.Do(request)
	if err != nil {
		return errors.New("owner probe did not reach the local application")
	}
	defer func() { _ = response.Body.Close() }() // Cleanup must not replace the operation result.
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return errors.New("owner probe response could not be read")
	}
	var protocol struct {
		Code      int    `json:"code"`
		Message   string `json:"msg"`
		TradeNo   string `json:"trade_no"`
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(responseBody, &protocol); err != nil {
		return errors.New("owner probe received an invalid protocol response")
	}
	if protocol.Message != "" && !regexp.MustCompile(`^[A-Z0-9_]{1,64}$`).MatchString(protocol.Message) {
		protocol.Message = "OTHER"
	}
	if protocol.ErrorCode != "" && !regexp.MustCompile(`^[A-Z0-9_]{1,64}$`).MatchString(protocol.ErrorCode) {
		protocol.ErrorCode = "OTHER"
	}
	if protocol.TradeNo != "" && !regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`).MatchString(protocol.TradeNo) {
		protocol.TradeNo = "OTHER"
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"http_status":         response.StatusCode,
		"local_address":       localAddress,
		"request_id":          response.Header.Get("X-Request-ID"),
		"protocol_code":       protocol.Code,
		"protocol_message":    protocol.Message,
		"protocol_error_code": protocol.ErrorCode,
		"trade_no":            protocol.TradeNo,
	})
}

func writeQRPNG(path, payload string) error {
	if path == "" || payload == "" || len(payload) > 1024 {
		return errors.New("QR output path and payload are required")
	}
	matrix, err := qrcode.NewQRCodeWriter().EncodeWithoutHint(payload, gozxing.BarcodeFormat_QR_CODE, 512, 512)
	if err != nil {
		return errors.New("payment QR fixture could not be encoded")
	}
	imageData := image.NewNRGBA(image.Rect(0, 0, matrix.GetWidth(), matrix.GetHeight()))
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	black := color.NRGBA{A: 255}
	for y := 0; y < matrix.GetHeight(); y++ {
		for x := 0; x < matrix.GetWidth(); x++ {
			pixel := white
			if matrix.Get(x, y) {
				pixel = black
			}
			imageData.SetNRGBA(x, y, pixel)
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("private QR fixture output could not be created")
	}
	if err := png.Encode(file, imageData); err != nil {
		_ = file.Close()
		return errors.New("private QR fixture output could not be encoded")
	}
	return file.Close()
}

func prepareMaterials(outputDir, rootBundlePath string) (fixtureManifest, error) {
	if outputDir == "" {
		return fixtureManifest{}, errors.New("fixture output directory is required")
	}
	rootBundle, err := os.ReadFile(rootBundlePath)
	if err != nil {
		return fixtureManifest{}, errors.New("system CA bundle is unavailable")
	}
	return writeFixtureMaterials(outputDir, rootBundle)
}

func writeFixtureMaterials(outputDir string, rootBundle []byte) (fixtureManifest, error) {
	if len(rootBundle) == 0 {
		return fixtureManifest{}, errors.New("system CA bundle is empty")
	}
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return fixtureManifest{}, err
	}
	if err := os.Chmod(outputDir, 0755); err != nil {
		return fixtureManifest{}, err
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fixtureManifest{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fixtureManifest{}, err
	}
	now := time.Now().UTC()
	caTemplate := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "VanillaPay local smoke CA"}, NotBefore: now.Add(-5 * time.Minute), NotAfter: now.Add(24 * time.Hour), KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature, BasicConstraintsValid: true, IsCA: true}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return fixtureManifest{}, err
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return fixtureManifest{}, err
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
	caKeyDER, err := x509.MarshalPKCS8PrivateKey(caKey)
	if err != nil {
		return fixtureManifest{}, err
	}
	material := map[string][]byte{"ca-a.pem": caPEM, "ca-a-key.pem": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: caKeyDER})}
	names := map[string][]string{
		"callback":  {callbackHost},
		"turnstile": {turnstileHost},
		"smtp":      {smtpHost},
		"nginx":     {"localhost", "127.0.0.1"},
	}
	fingerprints := make(map[string]string, len(names))
	for key, sans := range names {
		certPEM, keyPEM, leafDER, err := issueServerCertificate(caCert, caKey, sans, now.Add(-5*time.Minute), now.Add(24*time.Hour))
		if err != nil {
			return fixtureManifest{}, err
		}
		material[key+"-cert.pem"] = certPEM
		material[key+"-key.pem"] = keyPEM
		fingerprint := sha256.Sum256(leafDER)
		fingerprints[key] = hex.EncodeToString(fingerprint[:])
	}
	bundle := bytes.NewBuffer(append([]byte(nil), rootBundle...))
	if !bytes.HasSuffix(rootBundle, []byte("\n")) {
		bundle.WriteByte('\n')
	}
	bundle.Write(caPEM)
	material["ca-bundle.pem"] = bundle.Bytes()
	rootHash := sha256.Sum256(rootBundle)
	manifest := fixtureManifest{Version: 1, CreatedAt: now, CAFingerprint: fingerprintDER(caCert.Raw), Certificates: fingerprints, SANs: names, RootBundleHash: hex.EncodeToString(rootHash[:])}
	for name, contents := range material {
		path := filepath.Join(outputDir, name)
		mode := os.FileMode(0600)
		if strings.HasSuffix(name, "-cert.pem") || name == "ca-bundle.pem" {
			mode = 0644
		}
		if err := writeFileMode(path, contents, mode); err != nil {
			return fixtureManifest{}, err
		}
		manifest.MaterialFiles = append(manifest.MaterialFiles, name)
	}
	// Stable ordering makes this non-sensitive manifest easy to compare.
	sortStrings(manifest.MaterialFiles)
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fixtureManifest{}, err
	}
	if err := writeFileMode(filepath.Join(outputDir, "manifest.json"), append(encoded, '\n'), 0644); err != nil {
		return fixtureManifest{}, err
	}
	return manifest, nil
}

func issueServerCertificate(ca *x509.Certificate, caKey *ecdsa.PrivateKey, names []string, notBefore, notAfter time.Time) ([]byte, []byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, nil, err
	}
	leaf := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: names[0]}, NotBefore: notBefore, NotAfter: notAfter, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	for _, name := range names {
		if ip := net.ParseIP(name); ip != nil {
			leaf.IPAddresses = append(leaf.IPAddresses, ip)
		} else {
			leaf.DNSNames = append(leaf.DNSNames, name)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), der, nil
}

func serve(stateDir, stateFile, emailCodeFile, action, hostname string) error {
	callbackCert, err := tls.LoadX509KeyPair(filepath.Join(stateDir, "callback-cert.pem"), filepath.Join(stateDir, "callback-key.pem"))
	if err != nil {
		return errors.New("callback test certificate is unavailable")
	}
	turnstileCert, err := tls.LoadX509KeyPair(filepath.Join(stateDir, "turnstile-cert.pem"), filepath.Join(stateDir, "turnstile-key.pem"))
	if err != nil {
		return errors.New("siteverify test certificate is unavailable")
	}
	smtpCert, err := tls.LoadX509KeyPair(filepath.Join(stateDir, "smtp-cert.pem"), filepath.Join(stateDir, "smtp-key.pem"))
	if err != nil {
		return errors.New("SMTP test certificate is unavailable")
	}
	store := &stateStore{path: stateFile, emailCodePath: emailCodeFile, data: fixtureState{UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}}
	if err := store.persistLocked(); err != nil {
		return err
	}
	callback, err := startHTTPSEndpoint(callbackAddress, callbackCert, func(w http.ResponseWriter, r *http.Request) {
		store.addHTTPRequest(false, r)
		if r.URL.Path == "/crash-recover" && store.addCrashRecoveryRequest() == 1 {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(5 * time.Minute):
			}
		}
		if strings.Contains(r.URL.Path, "fail") {
			http.Error(w, "controlled callback failure", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "success")
	})
	if err != nil {
		return err
	}
	turnstile, err := startHTTPSEndpoint(turnstileAddress, turnstileCert, func(w http.ResponseWriter, r *http.Request) {
		store.addHTTPRequest(true, r)
		if r.Method != http.MethodPost || r.URL.Path != "/turnstile/v0/siteverify" {
			http.NotFound(w, r)
			return
		}
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		token := r.PostForm.Get("response")
		responseAction := action
		if prefix, value, ok := strings.Cut(token, ":"); ok && prefix == "fixture" && value != "" {
			responseAction = value
		}
		result := map[string]any{"success": token != "fixture-fail", "action": responseAction, "hostname": hostname}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	if err != nil {
		_ = callback.Close()
		store.setHealthy(false)
		return err
	}
	smtpListener, err := tls.Listen("tcp4", smtpAddress, &tls.Config{Certificates: []tls.Certificate{smtpCert}, MinVersion: tls.VersionTLS12, GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		store.addServerName(hello.ServerName)
		return nil, nil
	}})
	if err != nil {
		_ = callback.Close()
		_ = turnstile.Close()
		store.setHealthy(false)
		return errors.New("SMTP fixture listener could not bind")
	}
	store.setHealthy(true)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	serveSMTP(ctx, smtpListener, store)
	servers := []*http.Server{callback, turnstile}
	for _, server := range servers {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		_ = server.Shutdown(shutdownCtx)
		cancel()
	}
	_ = smtpListener.Close()
	store.setHealthy(false)
	return nil
}

func startHTTPSEndpoint(address string, certificate tls.Certificate, handler http.HandlerFunc) (*http.Server, error) {
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		return nil, fmt.Errorf("TLS fixture listener could not bind %s", address)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 3 * time.Second, TLSConfig: &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}}
	go func() { _ = server.Serve(tls.NewListener(listener, server.TLSConfig)) }()
	return server, nil
}

func serveSMTP(ctx context.Context, listener net.Listener, store *stateStore) {
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		connection, err := listener.Accept()
		if err != nil {
			return
		}
		go handleSMTP(connection, store)
	}
}

func handleSMTP(connection net.Conn, store *stateStore) {
	defer func() { _ = connection.Close() }() // Cleanup must not replace the operation result.
	if tlsConn, ok := connection.(*tls.Conn); ok {
		if err := tlsConn.Handshake(); err != nil {
			store.addSMTP(false)
			return
		}
	}
	reader := bufio.NewReader(connection)
	writer := bufio.NewWriter(connection)
	writeSMTPLine(writer, "220 smtp.vp.test ESMTP ready")
	authed := false
	accepted := false
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if accepted {
				store.addSMTP(true)
			} else {
				store.addSMTP(false)
			}
			return
		}
		command := strings.TrimSpace(line)
		upper := strings.ToUpper(command)
		switch {
		case strings.HasPrefix(upper, "EHLO ") || strings.HasPrefix(upper, "HELO "):
			_, _ = writer.WriteString("250-smtp.vp.test\r\n250-AUTH PLAIN LOGIN\r\n250 SIZE 1048576\r\n")
			_ = writer.Flush()
		case strings.HasPrefix(upper, "AUTH PLAIN "):
			credential := strings.TrimSpace(command[len("AUTH PLAIN "):])
			decoded, decodeErr := base64.StdEncoding.DecodeString(credential)
			authed = decodeErr == nil && len(decoded) > 0
			if authed {
				writeSMTPLine(writer, "235 2.7.0 authentication successful")
			} else {
				writeSMTPLine(writer, "535 5.7.8 authentication failed")
			}
		case strings.HasPrefix(upper, "AUTH LOGIN"):
			writeSMTPLine(writer, "502 5.7.0 only PLAIN is supported")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			if !authed {
				writeSMTPLine(writer, "530 5.7.0 authentication required")
			} else if _, err := mail.ParseAddress(extractSMTPAddress(command)); err != nil {
				writeSMTPLine(writer, "501 5.1.7 invalid sender")
			} else {
				writeSMTPLine(writer, "250 2.1.0 sender accepted")
			}
		case strings.HasPrefix(upper, "RCPT TO:"):
			if _, err := mail.ParseAddress(extractSMTPAddress(command)); err != nil {
				writeSMTPLine(writer, "501 5.1.3 invalid recipient")
			} else {
				writeSMTPLine(writer, "250 2.1.5 recipient accepted")
			}
		case upper == "DATA":
			writeSMTPLine(writer, "354 end with <CRLF>.<CRLF>")
			total := 0
			var message strings.Builder
			for {
				dataLine, readErr := reader.ReadString('\n')
				if readErr != nil {
					store.addSMTP(false)
					return
				}
				total += len(dataLine)
				if strings.TrimRight(dataLine, "\r\n") == "." {
					break
				}
				if total <= 1<<20 {
					message.WriteString(dataLine)
				}
				if total > 1<<20 {
					store.addSMTP(false)
					writeSMTPLine(writer, "552 5.3.4 message too large")
					return
				}
			}
			accepted = authed
			if accepted {
				store.captureEmailCode(message.String())
				writeSMTPLine(writer, "250 2.0.0 queued")
			} else {
				writeSMTPLine(writer, "530 5.7.0 authentication required")
			}
		case upper == "QUIT":
			writeSMTPLine(writer, "221 2.0.0 closing connection")
			if accepted {
				store.addSMTP(true)
			} else {
				store.addSMTP(false)
			}
			return
		case upper == "STARTTLS":
			writeSMTPLine(writer, "502 5.5.1 implicit TLS only")
		default:
			writeSMTPLine(writer, "250 2.0.0 ok")
		}
	}
}

func writeSMTPLine(writer *bufio.Writer, line string) {
	_, _ = writer.WriteString(line + "\r\n")
	_ = writer.Flush()
}

func extractSMTPAddress(command string) string {
	start := strings.IndexByte(command, '<')
	end := strings.IndexByte(command, '>')
	if start >= 0 && end > start {
		return command[start+1 : end]
	}
	return ""
}

func (s *stateStore) mutate(update func(*fixtureState)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.data)
	s.data.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_ = s.persistLocked()
}

func (s *stateStore) persistLocked() error {
	encoded, err := json.Marshal(s.data)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	temp := s.path + ".tmp"
	if err := os.WriteFile(temp, encoded, 0600); err != nil {
		return err
	}
	return os.Rename(temp, s.path)
}

func (s *stateStore) addHTTPRequest(siteverify bool, request *http.Request) {
	s.mutate(func(data *fixtureState) {
		if siteverify {
			data.Siteverify++
		} else {
			data.CallbackHTTP++
		}
		if request.TLS != nil && request.TLS.ServerName != "" {
			data.ServerNames = append(data.ServerNames, request.TLS.ServerName)
		}
	})
}

func (s *stateStore) addCrashRecoveryRequest() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data.CallbackCrashRequests++
	s.data.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	_ = s.persistLocked()
	return s.data.CallbackCrashRequests
}

func (s *stateStore) addServerName(name string) {
	s.mutate(func(data *fixtureState) { data.ServerNames = append(data.ServerNames, name) })
}

func (s *stateStore) addSMTP(accepted bool) {
	s.mutate(func(data *fixtureState) {
		if accepted {
			data.SMTPAccepted++
		} else {
			data.SMTPRejected++
		}
	})
}

func (s *stateStore) captureEmailCode(message string) {
	match := regexp.MustCompile(`(?i)verification code is ([0-9]{6})`).FindStringSubmatch(message)
	if len(match) != 2 || s.emailCodePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.emailCodePath), 0700); err != nil {
		return
	}
	_ = writePrivateFile(s.emailCodePath, []byte(match[1]+"\n"))
}

func (s *stateStore) setHealthy(healthy bool) {
	s.mutate(func(data *fixtureState) { data.Healthy = healthy })
}

func fingerprintDER(der []byte) string {
	hash := sha256.Sum256(der)
	return hex.EncodeToString(hash[:])
}

func writePrivateFile(path string, contents []byte) error {
	return writeFileMode(path, contents, 0600)
}

func writeFileMode(path string, contents []byte, mode os.FileMode) error {
	if err := os.WriteFile(path, contents, 0600); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func validateFixtureText(value string, max int) error {
	if strings.TrimSpace(value) == "" || len(value) > max || strings.ContainsAny(value, "\r\n\x00") {
		return errors.New("invalid fixture text")
	}
	return nil
}

func safeEndpointRecord(method, rawURL string, body []byte) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "invalid_endpoint"
	}
	path := parsed.EscapedPath()
	if path == "" {
		path = "/"
	}
	if len(path) > 256 {
		path = path[:256]
	}
	_ = body // request bodies, query values, credentials, and tokens never enter fixture evidence.
	return fmt.Sprintf("%s %s", strings.ToUpper(method), path)
}
