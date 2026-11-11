package settings

import (
	"context"
	"encoding/json"
	"testing"

	"vanillapay/internal/platform"
	"vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_LegalUpdatesCreateImmutableTermsAndPrivacyRevisions(t *testing.T) {
	db := testsupport.NewDatabase(t)
	application := db.NewApplication(t)
	value := json.RawMessage(`{"items":[],"terms":{"zh_CN":"条款一","en":""},"privacy":{"zh_CN":"隐私一","en":""}}`)
	actor := platform.Actor{Role: "admin", SubjectID: 1}
	if _, err := application.Services.Settings.UpdateGroup(context.Background(), actor, "footer_legal", settings.UpdateGroup{ExpectedVersion: 1, Value: value}); err != nil {
		t.Fatal(err)
	}
	value = json.RawMessage(`{"items":[],"terms":{"zh_CN":"条款二","en":"Terms two"},"privacy":{"zh_CN":"隐私二","en":"Privacy two"}}`)
	if _, err := application.Services.Settings.UpdateGroup(context.Background(), actor, "footer_legal", settings.UpdateGroup{ExpectedVersion: 2, Value: value}); err != nil {
		t.Fatal(err)
	}
	var terms, privacy int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE kind='terms'),count(*) FILTER (WHERE kind='privacy') FROM legal_versions`).Scan(&terms, &privacy); err != nil {
		t.Fatal(err)
	}
	if terms != 2 || privacy != 2 {
		t.Fatalf("legal revision counts terms=%d privacy=%d", terms, privacy)
	}
}

func TestDB_LegalRevisionsOnlyChangeForEditedLanguageText(t *testing.T) {
	db := testsupport.NewDatabase(t)
	service := db.NewApplication(t).Services.Settings
	ctx := context.Background()
	actor := platform.Actor{Role: "admin", SubjectID: 1}
	save := func(version int64, items, englishTerms string) {
		t.Helper()
		value := json.RawMessage(`{"items":` + items + `,"terms":{"zh_CN":"条款","en":"` + englishTerms +
			`"},"privacy":{"zh_CN":"隐私","en":"Privacy"}}`)
		_, err := service.UpdateGroup(ctx, actor, "footer_legal", settings.UpdateGroup{ExpectedVersion: version, Value: value})
		if err != nil {
			t.Fatal(err)
		}
	}
	assertCounts := func(wantTerms, wantPrivacy int) {
		t.Helper()
		var terms, privacy int
		err := db.Pool.QueryRow(ctx, `
			SELECT count(*) FILTER (WHERE kind='terms'), count(*) FILTER (WHERE kind='privacy') FROM legal_versions
		`).Scan(&terms, &privacy)
		if err != nil {
			t.Fatal(err)
		}
		if terms != wantTerms || privacy != wantPrivacy {
			t.Fatalf("legal revision counts terms=%d privacy=%d, want terms=%d privacy=%d", terms, privacy, wantTerms, wantPrivacy)
		}
	}

	const footer = `[{"id":"help","label":{"zh_CN":"帮助","en":"Help"},"text":{"zh_CN":"查看帮助","en":"Read the guide"},"url":"https://example.test","enabled":true,"sort":0}]`
	save(1, "[]", "Terms")
	assertCounts(1, 1)
	save(2, footer, "Terms")
	assertCounts(1, 1)
	save(3, footer, "Terms")
	assertCounts(1, 1)
	save(4, footer, "Revised terms")
	assertCounts(2, 1)
}
