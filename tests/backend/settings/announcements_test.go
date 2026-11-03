package settings

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_AnnouncementRevisionIsShownAgainUntilRead(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	ctx := context.Background()
	admin := platform.Actor{Role: "admin", SubjectID: 1}
	created, err := application.Services.Portal.SaveAnnouncement(ctx, admin, portal.AnnouncementCommand{Title: portal.LocalizedText{ZhCN: "维护", En: "Maintenance"}, Body: portal.LocalizedText{ZhCN: "今晚维护", En: "Maintenance tonight"}, Popup: true})
	if err != nil {
		t.Fatal(err)
	}
	merchantActor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	page, err := application.Services.Portal.ListNotifications(ctx, merchantActor, portal.PageQuery{Limit: 20})
	if err != nil || len(page.PopupAnnouncements) != 1 || page.PopupAnnouncements[0].Revision != created.Revision {
		t.Fatalf("initial popup list = %#v, %v", page.PopupAnnouncements, err)
	}
	t.Run("V22.B.r02/v01", func(t *testing.T) {
		if len(page.PopupAnnouncements) != 1 || page.PopupAnnouncements[0].Read || page.PopupAnnouncements[0].Revision != created.Revision {
			t.Fatalf("first unread popup announcement = %#v", page.PopupAnnouncements)
		}
	})
	if err := application.Services.Portal.MarkAnnouncementRead(ctx, merchantActor, created.ID, created.Revision); err != nil {
		t.Fatal("mark original popup revision read")
	}
	t.Run("V22.B.r02/v02", func(t *testing.T) {
		readPage, readErr := application.Services.Portal.ListNotifications(ctx, merchantActor, portal.PageQuery{Limit: 20})
		if readErr != nil || len(readPage.PopupAnnouncements) != 0 {
			t.Fatalf("read popup revision remained visible: %#v err=%v", readPage.PopupAnnouncements, readErr)
		}
	})
	updated, err := application.Services.Portal.SaveAnnouncement(ctx, admin, portal.AnnouncementCommand{ID: created.ID, Revision: created.Revision, Title: created.Title, Body: portal.LocalizedText{ZhCN: "今晚维护延期", En: "Maintenance delayed"}, Popup: true})
	if err != nil || updated.Revision != created.Revision+1 {
		t.Fatalf("announcement revision update = %#v, %v", updated, err)
	}
	t.Run("V22.B.r02/v03", func(t *testing.T) {
		updatedPage, updateErr := application.Services.Portal.ListNotifications(ctx, merchantActor, portal.PageQuery{Limit: 20})
		if updateErr != nil || len(updatedPage.PopupAnnouncements) != 1 || updatedPage.PopupAnnouncements[0].Revision != updated.Revision || updatedPage.PopupAnnouncements[0].Read {
			t.Fatalf("new announcement revision did not reappear unread: %#v err=%v", updatedPage.PopupAnnouncements, updateErr)
		}
	})
	if err := application.Services.Portal.MarkAnnouncementRead(ctx, merchantActor, updated.ID, updated.Revision); err != nil {
		t.Fatal(err)
	}
	page, err = application.Services.Portal.ListNotifications(ctx, merchantActor, portal.PageQuery{Limit: 20})
	if err != nil || len(page.PopupAnnouncements) != 0 {
		t.Fatalf("read announcement remained in popup list: %#v, %v", page.PopupAnnouncements, err)
	}
	if err := application.Services.Portal.DeleteAnnouncement(ctx, admin, updated.ID, updated.Revision); err != nil {
		t.Fatalf("soft-delete announcement: %v", err)
	}
	t.Run("V22.B.r02/v04", func(t *testing.T) {
		deletedPage, listErr := application.Services.Portal.ListNotifications(ctx, merchantActor, portal.PageQuery{Limit: 20})
		if listErr != nil || len(deletedPage.PopupAnnouncements) != 0 {
			t.Fatalf("deleted announcement remained in popup list: %#v err=%v", deletedPage.PopupAnnouncements, listErr)
		}
		adminPage, adminErr := application.Services.Portal.ListAnnouncements(ctx, admin, portal.PageQuery{Limit: 20})
		if adminErr != nil || len(adminPage.Items) != 0 || adminPage.Total != 0 {
			t.Fatalf("deleted announcement remained visible in admin list: %#v err=%v", adminPage, adminErr)
		}
	})
}

func TestDB_NotificationReadCountsRespectMerchantOwnership(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantA, _ := db.SeedMerchant(t)
	merchantB, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	ctx := context.Background()
	insert := func(id string, merchantID int64, kind string) {
		t.Helper()
		payload := json.RawMessage(`{"title":{"zh_CN":"提醒","en":"Notice"},"summary":{"zh_CN":"摘要","en":"Summary"}}`)
		if _, err := db.Pool.Exec(ctx, `INSERT INTO notifications(public_id,merchant_id,kind,source_id,dedupe_key,safe_payload) VALUES($1,$2,$3,$4,$5,$6)`, id, merchantID, kind, "source-"+id, "dedupe-"+id, payload); err != nil {
			t.Fatalf("seed %s notification for merchant %d: %v", kind, merchantID, err)
		}
	}
	insert("merchant-a-refund", merchantA, "refund_request")
	insert("merchant-a-offline", merchantA, "device_offline")
	insert("merchant-b-refund", merchantB, "refund_request")
	actorA := platform.Actor{Role: "merchant", SubjectID: merchantA, MerchantID: merchantA}
	t.Run("V22.B.r03/v01", func(t *testing.T) {
		page, err := application.Services.Portal.ListNotifications(ctx, actorA, portal.PageQuery{Limit: 20, UnreadOnly: true})
		if err != nil || page.Total != 2 || page.UnreadCount != 2 || len(page.Items) != 2 {
			t.Fatalf("unread refund/offline notifications page=%#v err=%v", page, err)
		}
		kinds := map[string]bool{}
		for _, item := range page.Items {
			kinds[item.Kind] = true
		}
		if !kinds["refund_request"] || !kinds["device_offline"] {
			t.Fatalf("unread notification kinds=%#v", kinds)
		}
	})
	t.Run("V22.B.r03/v03", func(t *testing.T) {
		result, err := application.Services.Portal.MarkRead(ctx, actorA, portal.ReadRequest{IDs: []string{"merchant-b-refund"}})
		if err != nil || result.UnreadCount != 2 {
			t.Fatalf("cross-merchant mark-read result=%#v err=%v, want no state change", result, err)
		}
		var readAt *time.Time
		if err := db.Pool.QueryRow(ctx, `SELECT read_at FROM notifications WHERE public_id='merchant-b-refund'`).Scan(&readAt); err != nil || readAt != nil {
			t.Fatalf("cross-merchant read changed merchant B row: read_at=%v err=%v", readAt, err)
		}
	})
	t.Run("V22.B.r03/v02", func(t *testing.T) {
		one, err := application.Services.Portal.MarkRead(ctx, actorA, portal.ReadRequest{IDs: []string{"merchant-a-refund"}})
		if err != nil || one.UnreadCount != 1 {
			t.Fatalf("single notification read result=%#v err=%v, want one unread", one, err)
		}
		all, err := application.Services.Portal.MarkRead(ctx, actorA, portal.ReadRequest{All: true})
		if err != nil || all.UnreadCount != 0 {
			t.Fatalf("mark all notifications read result=%#v err=%v, want zero unread", all, err)
		}
	})
}

func TestDB_AnnouncementsStayOutOfPublicConfigAndMailJobs(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	ctx := context.Background()
	admin := platform.Actor{Role: "admin", SubjectID: 1}
	merchantActor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	created, err := application.Services.Portal.SaveAnnouncement(ctx, admin, portal.AnnouncementCommand{
		Title: portal.LocalizedText{ZhCN: "公告哨兵", En: "announcement sentinel"},
		Body:  portal.LocalizedText{ZhCN: "只给商户", En: "merchant notice only"}, Popup: true,
	})
	if err != nil {
		t.Fatal("create merchant announcement")
	}
	t.Run("V22.B.r01/v01", func(t *testing.T) {
		page, listErr := application.Services.Portal.ListNotifications(ctx, merchantActor, portal.PageQuery{Limit: 20})
		if listErr != nil || len(page.PopupAnnouncements) != 1 || page.PopupAnnouncements[0].ID != created.ID {
			t.Fatalf("merchant notifications did not contain published announcement: %#v err=%v", page, listErr)
		}
		var announcementRows int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE merchant_id=$1 AND kind='announcement'`, merchantID).Scan(&announcementRows); err != nil || announcementRows != 0 {
			t.Fatalf("announcement was duplicated as a regular notification row: rows=%d err=%v", announcementRows, err)
		}
	})
	t.Run("V22.B.r01/v02", func(t *testing.T) {
		public, publicErr := application.Services.Settings.GetPublic(ctx)
		if publicErr != nil {
			t.Fatal("read public configuration")
		}
		encoded, err := json.Marshal(public)
		if err != nil || strings.Contains(string(encoded), "announcement sentinel") || strings.Contains(string(encoded), "只给商户") {
			t.Fatalf("public config exposed announcement text: %s err=%v", encoded, err)
		}
	})
	t.Run("V22.B.r01/v03", func(t *testing.T) {
		var jobs int
		if err := db.Pool.QueryRow(ctx, `SELECT count(*) FROM mail_deliveries WHERE template LIKE '%announcement%'`).Scan(&jobs); err != nil || jobs != 0 {
			t.Fatalf("announcement publication created %d announcement mail jobs, err=%v", jobs, err)
		}
	})
}
