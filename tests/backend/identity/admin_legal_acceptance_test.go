package identity

import (
	"context"
	"encoding/json"
	"testing"

	"vanillapay/internal/merchant"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_AdminCreatedMerchantDoesNotForgeLegalAcceptance(t *testing.T) {
	db := testsupport.NewDatabase(t)
	application := db.NewApplication(t)
	ctx := context.Background()
	if _, err := db.Pool.Exec(ctx, `INSERT INTO legal_versions(kind,revision,content) VALUES('terms',1,$1),('privacy',1,$1)`, json.RawMessage(`{"zh_CN":"terms","en":"terms"}`)); err != nil {
		t.Fatal("seed current legal revisions")
	}
	t.Run("V21.B.r02/v03", func(t *testing.T) {
		created, err := application.Services.Merchant.Create(ctx, platform.Actor{Role: "admin", SubjectID: 1}, merchant.CreateMerchant{
			Username: "admin_created_without_consent", Email: "admin-created@example.test", Password: fixturePassword, PasswordConfirmation: fixturePassword,
		})
		if err != nil || created.ID == "" {
			t.Fatalf("admin-created merchant=%#v err=%v", created, err)
		}
		var acceptances int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM legal_acceptances WHERE merchant_id=$1`, created.ID).Scan(&acceptances); err != nil || acceptances != 0 {
			t.Fatalf("admin-created merchant has %d fabricated legal acceptance rows, err=%v", acceptances, err)
		}
	})
}
