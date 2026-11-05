// Browser-test server: production router/services and PostgreSQL, with only
// Siteverify redirected to a local HTTP fixture. Never compiled into cmd/server.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"
	"vanillapay/internal/app"
	"vanillapay/internal/auth"
	"vanillapay/internal/config"
	dbmigrate "vanillapay/internal/db/migrate"
	"vanillapay/internal/httpx"
	"vanillapay/internal/platform"
	"vanillapay/internal/settings"
)

type localSiteverify struct{ target *url.URL }

func (s localSiteverify) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != "challenges.cloudflare.com" || r.URL.Path != "/turnstile/v0/siteverify" {
		return nil, errors.New("unexpected external request in browser tests")
	}
	clone := r.Clone(r.Context())
	clone.URL.Scheme, clone.URL.Host = s.target.Scheme, s.target.Host
	clone.Host = s.target.Host
	return http.DefaultTransport.RoundTrip(clone)
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := run(ctx); err != nil {
		log.Fatal("browser-test server failed") // Never log database URLs or fixture credentials.
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	host, _, err := net.SplitHostPort(cfg.ListenAddr)
	if err != nil || (host != "localhost" && host != "127.0.0.1") || cfg.AppEnv != "development" {
		return errors.New("browser-test server requires a development loopback address")
	}
	if err := dbmigrate.Up(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	secret, err := platform.RandomToken(rand.Reader, 32)
	if err != nil {
		return err
	}
	token, err := platform.RandomToken(rand.Reader, 32)
	if err != nil {
		return err
	}
	publicURL, err := url.Parse(cfg.PublicBaseURL)
	if err != nil {
		return err
	}
	siteverify := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": r.PostForm.Get("secret") == secret && r.PostForm.Get("response") == token, "action": "login", "hostname": publicURL.Hostname()})
	}))
	defer siteverify.Close()
	target, err := url.Parse(siteverify.URL)
	if err != nil {
		return err
	}
	application, err := app.NewWithDependencies(ctx, cfg, app.Dependencies{DatabaseFactory: func(context.Context, config.Config) (app.Database, error) { return pool, nil }})
	if err != nil {
		return err
	}
	defer func() { _ = application.Close() }() // Cleanup must not replace a startup or serve error.
	// Capture runtime verification mail in memory; SMTP is tested separately.
	inbox := &browserInbox{codes: make(map[string]string)}
	application.Services.Auth = auth.NewService(pool, cfg.EncryptionKey, nil, nil, inbox, auth.Siteverify{Client: &http.Client{Transport: localSiteverify{target: target}, Timeout: 5 * time.Second}})
	value, err := json.Marshal(map[string]any{"turnstile_site_key": token, "turnstile_secret": secret, "allowed_hostname": publicURL.Hostname(), "restrict_email_domains": false, "email_domains": []string{}, "ban_contact": map[string]string{"zh_CN": "请联系平台管理员", "en": "Contact the platform administrator"}})
	if err != nil {
		return err
	}
	var version int64
	if err := pool.QueryRow(ctx, `SELECT version FROM site_settings WHERE singleton=true`).Scan(&version); err != nil {
		return err
	}
	if _, err := application.Services.Settings.UpdateGroup(ctx, platform.Actor{Role: "admin"}, "security", settings.UpdateGroup{ExpectedVersion: version, Value: value}); err != nil {
		return err
	}
	mux := http.NewServeMux()
	inbox.register(mux)
	mux.HandleFunc("POST /__e2e/merchant", func(w http.ResponseWriter, r *http.Request) {
		var flags struct {
			TOTP      bool   `json:"totp"`
			Banned    bool   `json:"banned"`
			BanReason string `json:"ban_reason"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&flags) != nil {
			http.Error(w, "invalid fixture options", http.StatusBadRequest)
			return
		}
		fixture, err := seedMerchant(r.Context(), pool, cfg.EncryptionKey, flags.TOTP, flags.Banned, flags.BanReason)
		if err != nil {
			http.Error(w, "merchant fixture failed", http.StatusInternalServerError)
			return
		}
		fixture["turnstileToken"] = token
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(fixture)
	})
	registerOverviewFixtures(mux, pool)
	mux.Handle("/", httpx.NewRouter(application))
	return application.Start(ctx, mux)
}

func seedMerchant(ctx context.Context, pool *pgxpool.Pool, key [32]byte, factor, banned bool, banReason string) (map[string]string, error) {
	if banReason == "" {
		banReason = "Browser test restriction"
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, err
	}
	username := "browser_" + hex.EncodeToString(random[:8])
	password, err := platform.RandomToken(rand.Reader, 32)
	if err != nil {
		return nil, err
	}
	hash, err := auth.HashPassword(password, rand.Reader)
	if err != nil {
		return nil, err
	}
	pidNumber, err := rand.Int(rand.Reader, big.NewInt(1_000_000_000_000_000))
	if err != nil {
		return nil, err
	}
	pid := fmt.Sprintf("8%015d", pidNumber.Int64())
	cipher, err := platform.Encrypt(key[:], []byte(hex.EncodeToString(random[:])), rand.Reader)
	if err != nil {
		return nil, err
	}
	var totpCipher []byte
	var seed string
	if factor {
		otp, err := totp.Generate(totp.GenerateOpts{Issuer: "Browser test", AccountName: username, Rand: rand.Reader})
		if err != nil {
			return nil, err
		}
		seed = otp.Secret()
		totpCipher, err = platform.Encrypt(key[:], []byte(seed), rand.Reader)
		if err != nil {
			return nil, err
		}
	}
	_, err = pool.Exec(ctx, `INSERT INTO merchants(username,username_normalized,email,email_normalized,pid,password_hash,key_cipher,totp_cipher,banned_at,ban_reason) VALUES($1,$1,$2,$2,$3,$4,$5,$6,CASE WHEN $7 THEN now() END,CASE WHEN $7 THEN $8 END)`, username, username+"@example.test", pid, hash, cipher, totpCipher, banned, banReason)
	if err != nil {
		return nil, err
	}
	return map[string]string{"username": username, "password": password, "pid": pid, "totpSeed": seed}, nil
}
