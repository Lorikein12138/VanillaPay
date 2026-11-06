package main

import (
	"crypto/rand"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pquerna/otp/totp"
	"vanillapay/internal/auth"
	"vanillapay/internal/platform"
)

// Only registered by the loopback-only E2E executable. The console specs run
// serially in one viewport project because the administrator is a singleton.
func registerConsoleFixtures(mux *http.ServeMux, pool *pgxpool.Pool, key [32]byte, service *auth.Service, turnstileToken string) {
	mux.HandleFunc("POST /__e2e/console", func(w http.ResponseWriter, r *http.Request) {
		var flags struct {
			Reset bool `json:"reset"`
			TOTP  bool `json:"totp"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024)).Decode(&flags) != nil {
			http.Error(w, "invalid console fixture options", 400)
			return
		}
		password, err := platform.RandomToken(rand.Reader, 32)
		if err != nil {
			http.Error(w, "console fixture failed", 500)
			return
		}
		hash, err := auth.HashPassword(password, rand.Reader)
		if err != nil {
			http.Error(w, "console fixture failed", 500)
			return
		}
		var cipher []byte
		var seed string
		if flags.TOTP {
			otp, err := totp.Generate(totp.GenerateOpts{Issuer: "Browser test", AccountName: "console_admin", Rand: rand.Reader})
			if err != nil {
				http.Error(w, "console fixture failed", 500)
				return
			}
			seed = otp.Secret()
			cipher, err = platform.Encrypt(key[:], []byte(seed), rand.Reader)
			if err != nil {
				http.Error(w, "console fixture failed", 500)
				return
			}
		}
		tx, err := pool.Begin(r.Context())
		if err != nil {
			http.Error(w, "console fixture failed", 500)
			return
		}
		defer func() { _ = tx.Rollback(r.Context()) }()
		for _, statement := range []string{`SELECT pg_advisory_xact_lock(710062)`, `DELETE FROM sessions WHERE role='admin'`, `DELETE FROM auth_challenges WHERE role='admin'`, `DELETE FROM recovery_codes WHERE role='admin'`, `TRUNCATE admins RESTART IDENTITY`} {
			if _, err := tx.Exec(r.Context(), statement); err != nil {
				http.Error(w, "console fixture reset failed", 500)
				return
			}
		}
		if !flags.Reset {
			if _, err := tx.Exec(r.Context(), `INSERT INTO admins(username,username_normalized,password_hash,totp_cipher) VALUES('console_admin','console_admin',$1,$2)`, hash, cipher); err != nil {
				http.Error(w, "console fixture failed", 500)
				return
			}
		}
		if err := tx.Commit(r.Context()); err != nil {
			http.Error(w, "console fixture failed", 500)
			return
		}
		result := map[string]string{"username": "console_admin", "password": password, "totpSeed": seed, "turnstileToken": turnstileToken}
		if flags.Reset {
			code, err := service.RotateBootstrapCode(r.Context())
			if err != nil {
				http.Error(w, "console bootstrap fixture failed", 500)
				return
			}
			result = map[string]string{"initialization_code": code}
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	})
	mux.HandleFunc("POST /__e2e/console/recovery", func(w http.ResponseWriter, r *http.Request) {
		code, err := service.IssueConsoleRecoveryCode(r.Context())
		if err != nil {
			http.Error(w, "console recovery fixture failed", 500)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"recovery_code": code})
	})
}
