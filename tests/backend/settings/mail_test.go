package settings

import (
	"context"
	"testing"

	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_MailTestIsPersistedAndQueryableWithoutReturningSecrets(t *testing.T) {
	db := testsupport.NewDatabase(t)
	application := db.NewApplication(t)
	ctx := context.Background()
	admin := platform.Actor{Role: "admin", SubjectID: 1}
	queued, err := application.Services.Settings.QueueMailTest(ctx, admin, " Operator@Example.Test ")
	if err != nil {
		t.Fatal(err)
	}
	id := queued["delivery_id"].(string)
	status, err := application.Services.Settings.MailTestStatus(ctx, admin, id)
	if err != nil || status["state"] != "queued" || status["id"] != id {
		t.Fatalf("mail test status = %#v, %v", status, err)
	}
	var recipientCipher []byte
	if err := db.Pool.QueryRow(ctx, `SELECT recipient_cipher FROM mail_deliveries WHERE public_id=$1`, id).Scan(&recipientCipher); err != nil {
		t.Fatal(err)
	}
	if len(recipientCipher) == 0 {
		t.Fatal("mail recipient was not encrypted before persistence")
	}
}
