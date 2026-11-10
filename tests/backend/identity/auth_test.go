package identity

import (
	"context"
	"testing"

	"vanillapay/internal/auth"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_BootstrapConsumesTheInitializationCodeOnce(t *testing.T) {
	db := testsupport.NewDatabase(t)
	service := auth.NewService(db.Pool, db.Key, db.Clock, nil, platform.DisabledMailer{}, nil, "")
	ctx := context.Background()
	code, err := service.RotateBootstrapCode(ctx)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Bootstrap(ctx, auth.BootstrapRequest{InitializationCode: code, Username: "operator", Password: fixturePassword, PasswordConfirmation: fixturePassword})
	if err != nil {
		t.Fatal(err)
	}
	if result.User.Role != "admin" || result.NextPath != "/console" || result.Token == "" || result.CSRFToken == "" {
		t.Fatalf("bootstrap session = %#v", result)
	}
	if _, err := service.Bootstrap(ctx, auth.BootstrapRequest{InitializationCode: code, Username: "operator2", Password: fixturePassword, PasswordConfirmation: fixturePassword}); err == nil {
		t.Fatal("second bootstrap was accepted")
	}
}
