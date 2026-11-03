package config

import (
	"os"
	"strings"
	"testing"
)

func TestUnit_LoadRequiresConfigurationWithoutEchoingValues(t *testing.T) {
	t.Run("V02.B.r04/v01", func(t *testing.T) {
		for _, name := range []string{"DATABASE_URL", "PUBLIC_BASE_URL", "APP_ENCRYPTION_KEY", "LISTEN_ADDR", "DATA_DIR", "TRUSTED_PROXY_CIDRS"} {
			unsetEnvironment(t, name)
		}
		t.Setenv("APP_ENV", "development")
		if _, err := Load(); err == nil {
			t.Fatal("Load() succeeded with missing required configuration")
		} else {
			for _, name := range []string{"DATABASE_URL", "PUBLIC_BASE_URL", "APP_ENCRYPTION_KEY", "LISTEN_ADDR", "DATA_DIR", "TRUSTED_PROXY_CIDRS"} {
				if !strings.Contains(err.Error(), name) {
					t.Errorf("error %q does not name %s", err, name)
				}
			}
		}
	})

	t.Run("V02.B.r04/v02", func(t *testing.T) {
		for _, name := range []string{"DATABASE_URL", "PUBLIC_BASE_URL", "APP_ENCRYPTION_KEY", "LISTEN_ADDR", "DATA_DIR", "TRUSTED_PROXY_CIDRS"} {
			unsetEnvironment(t, name)
		}
		t.Setenv("APP_ENV", "development")
		t.Setenv("APP_ENCRYPTION_KEY", "DO_NOT_PRINT_THIS_SECRET")
		_, err := Load()
		if err == nil {
			t.Fatal("Load() succeeded with missing required configuration")
		}
		if strings.Contains(err.Error(), "DO_NOT_PRINT_THIS_SECRET") {
			t.Fatalf("configuration error leaked a secret value: %q", err)
		}
	})
}

func unsetEnvironment(t *testing.T, name string) {
	t.Helper()
	previous, existed := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unset %s: %v", name, err)
	}
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv(name, previous)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

func TestUnit_LoadDevelopmentConfiguration(t *testing.T) {
	setValidEnvironment(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}
	if cfg.AppEnv != "development" || cfg.PublicBaseURL != "http://localhost:8080" {
		t.Fatalf("unexpected development config: %#v", cfg)
	}
	if got := len(cfg.TrustedProxyCIDRs); got != 2 {
		t.Fatalf("got %d trusted proxy CIDRs, want 2", got)
	}
	if got := cfg.EncryptionKey[0]; got != 0 {
		t.Fatalf("got first key byte %d, want zero", got)
	}
}

func TestUnit_LoadProductionRequiresHTTPSAndValidKey(t *testing.T) {
	setValidEnvironment(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("PUBLIC_BASE_URL", "http://localhost:8080")
	t.Setenv("APP_ENCRYPTION_KEY", "DO_NOT_PRINT_THIS_SECRET")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() succeeded with an insecure public URL and invalid key")
	}
	message := err.Error()
	if !strings.Contains(message, "PUBLIC_BASE_URL") || !strings.Contains(message, "APP_ENCRYPTION_KEY") {
		t.Fatalf("error did not identify invalid fields: %q", message)
	}
	if strings.Contains(message, "DO_NOT_PRINT_THIS_SECRET") {
		t.Fatalf("configuration error leaked a secret value: %q", message)
	}
}

func setValidEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("APP_ENV", "development")
	t.Setenv("DATABASE_URL", "postgres://vanillapay@127.0.0.1:5432/vanillapay?sslmode=disable")
	t.Setenv("PUBLIC_BASE_URL", "http://localhost:8080")
	t.Setenv("APP_ENCRYPTION_KEY", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=")
	t.Setenv("LISTEN_ADDR", "127.0.0.1:8080")
	t.Setenv("DATA_DIR", ".local/data")
	t.Setenv("TRUSTED_PROXY_CIDRS", "127.0.0.1/32, ::1/128")
}
