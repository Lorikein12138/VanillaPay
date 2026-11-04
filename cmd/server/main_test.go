package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"os"
	"testing"
	"time"

	"vanillapay/internal/auth"
	"vanillapay/tests/backend/testsupport"
)

func TestCommand_CodeOutputFailureIsReturned(t *testing.T) {
	for _, command := range []string{"serve", "console-recovery"} {
		t.Run(command, func(t *testing.T) {
			db := testsupport.NewDatabase(t)
			t.Setenv("APP_ENV", "production")
			t.Setenv("DATABASE_URL", db.URL)
			t.Setenv("APP_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(db.Key[:]))
			t.Setenv("PUBLIC_BASE_URL", "https://pay.example.test")
			t.Setenv("LISTEN_ADDR", "127.0.0.1:18799")
			t.Setenv("DATA_DIR", t.TempDir())
			t.Setenv("TRUSTED_PROXY_CIDRS", "")
			if command == "console-recovery" {
				application := db.NewApplication(t)
				code, err := application.Services.Auth.RotateBootstrapCode(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				password := "Aa1!" + rand.Text()
				_, err = application.Services.Auth.Bootstrap(t.Context(), auth.BootstrapRequest{InitializationCode: code, Username: "output_failure_admin", Password: password, PasswordConfirmation: password})
				if err != nil {
					t.Fatal(err)
				}
			}
			// A read-only file makes the actual fmt.Fprintf fail on Windows
			// and Linux, without changing the command or exposing its code.
			readOnly, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			original := os.Stdout
			os.Stdout = readOnly
			t.Cleanup(func() { os.Stdout = original; _ = readOnly.Close() })
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			err = run(ctx, []string{command})
			var writeError *os.PathError
			if !errors.As(err, &writeError) || writeError.Op != "write" {
				t.Fatalf("command must return its output error, got %T", err)
			}
		})
	}
}
