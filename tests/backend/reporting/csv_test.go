package reporting

import (
	"bytes"
	"context"
	"encoding/csv"
	"strings"
	"testing"
	"time"

	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_ExportUsesListFiltersFormulaEscapingAndAuditsOutcome(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	otherID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	created := db.Clock.Now()
	insertOrder(t, db, merchantID, "export-safe", "safe item", "paid", "alipay", created, datePtr(created), nil, 100)
	insertOrder(t, db, merchantID, "export-formula", "=HYPERLINK(\"x\")", "paid", "wxpay", created.Add(time.Second), datePtr(created.Add(time.Second)), nil, 200)
	insertOrder(t, db, merchantID, "export-pending", "pending item", "pending", "", created.Add(2*time.Second), nil, nil, 300)
	insertOrder(t, db, merchantID, "export-formula-plus", "+SUM(1,1)", "paid", "alipay", created.Add(3*time.Second), datePtr(created.Add(3*time.Second)), nil, 201)
	insertOrder(t, db, merchantID, "export-formula-minus", "-1+1", "paid", "alipay", created.Add(4*time.Second), datePtr(created.Add(4*time.Second)), nil, 202)
	insertOrder(t, db, merchantID, "export-formula-at", "@IMPORTXML(\"x\")", "paid", "alipay", created.Add(5*time.Second), datePtr(created.Add(5*time.Second)), nil, 203)
	insertOrder(t, db, merchantID, "export-quoted", "comma, line\nwith \"quotes\"", "paid", "alipay", created.Add(6*time.Second), datePtr(created.Add(6*time.Second)), nil, 204)
	insertOrder(t, db, otherID, "other-export", "private item", "paid", "alipay", created, datePtr(created), nil, 400)

	var output bytes.Buffer
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID, RequestID: "report-export-test"}
	if err := application.Services.Portal.ExportOrders(context.Background(), actor, portal.OrderFilter{Status: "paid", Limit: 1}, &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(output.Bytes(), []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("CSV is missing UTF-8 BOM")
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(output.String(), "\uFEFF")))
	rows, err := reader.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 7 || rows[5][0] != "export-formula" || rows[5][5] != "'=HYPERLINK(\"x\")" || rows[6][0] != "export-safe" {
		t.Fatalf("filtered CSV rows = %#v", rows)
	}
	if len(rows[0]) != 16 || strings.Contains(output.String(), "private item") || strings.Contains(output.String(), "clientip") {
		t.Fatalf("CSV exposed unexpected columns or another merchant: %#v", rows[0])
	}
	t.Run("V48.B.r02/v01", func(t *testing.T) {
		if rows[1][0] != "export-quoted" || rows[1][5] != "comma, line\nwith \"quotes\"" {
			t.Fatalf("CSV comma/newline/quote field did not round-trip: %#v", rows[1])
		}
	})
	t.Run("V48.B.r02/v02", func(t *testing.T) {
		want := map[string]string{"export-formula-plus": "'+SUM(1,1)", "export-formula-minus": "'-1+1", "export-formula-at": "'@IMPORTXML(\"x\")"}
		for _, row := range rows[1:] {
			if expected, ok := want[row[0]]; ok && row[5] != expected {
				t.Errorf("formula-like CSV text %q = %q, want %q", row[0], row[5], expected)
			}
		}
	})
	var audits int
	if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE merchant_id=$1 AND action='orders.export' AND request_id=$2 AND result='success'`, merchantID, actor.RequestID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("export audit count=%d err=%v", audits, err)
	}
}

func TestDB_ExportReturnsWriterFailureAndRecordsIt(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID, RequestID: "report-export-failure"}
	err := application.Services.Portal.ExportOrders(context.Background(), actor, portal.OrderFilter{}, failingWriter{})
	if err == nil {
		t.Fatal("writer failure was not returned")
	}
	var result string
	if err := db.Pool.QueryRow(context.Background(), `SELECT result FROM audit_logs WHERE merchant_id=$1 AND action='orders.export' AND request_id=$2`, merchantID, actor.RequestID).Scan(&result); err != nil || result != "failure" {
		t.Fatalf("export failure audit result=%q err=%v", result, err)
	}
}

func TestDB_ExportCancellationRecordsFailureWithPreservedRequestMetadata(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	application := db.NewApplication(t)
	metadata := platform.WithRequestMetadata(context.Background(), platform.RequestMetadata{IP: "::ffff:192.0.2.44", RequestID: "report-export-cancelled"})
	ctx, cancel := context.WithCancel(metadata)
	cancel()
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	t.Run("V16.B.r01/v12", func(t *testing.T) {
		var output bytes.Buffer
		if err := application.Services.Portal.ExportOrders(ctx, actor, portal.OrderFilter{}, &output); err == nil {
			t.Fatal("cancelled export unexpectedly succeeded")
		}
		var count int
		var result, ip, requestID string
		if err := db.Pool.QueryRow(context.Background(), `SELECT count(*),COALESCE(min(result),''),COALESCE(min(ip::text),''),COALESCE(min(request_id),'') FROM audit_logs WHERE merchant_id=$1 AND action='orders.export' AND request_id=$2`, merchantID, "report-export-cancelled").Scan(&count, &result, &ip, &requestID); err != nil {
			t.Fatalf("read cancellation audit: %v", err)
		}
		if count != 1 || result != "failure" || ip != "192.0.2.44/32" || requestID != "report-export-cancelled" {
			t.Fatalf("cancelled export audit count=%d result=%q ip=%q request_id=%q", count, result, ip, requestID)
		}
	})
	t.Run("V48.B.r04/v01", func(t *testing.T) {
		var count int
		if err := db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE request_id='report-export-cancelled' AND action='orders.export' AND result='failure'`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("cancelled export audit count=%d err=%v", count, err)
		}
	})
	t.Run("V48.B.r04/v02", func(t *testing.T) {
		var output bytes.Buffer
		if err := application.Services.Portal.ExportOrders(ctx, actor, portal.OrderFilter{}, &output); err == nil {
			t.Fatal("cancelled export unexpectedly succeeded")
		}
		if output.Len() != 0 {
			t.Fatalf("cancelled export returned a partial file (%d bytes)", output.Len())
		}
	})
}

func TestDB_ExportMatchesFilteredListsAndIncludesRefundedOrders(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantOne, _ := db.SeedMerchant(t)
	merchantTwo, _ := db.SeedMerchant(t)
	app := db.NewApplication(t)
	now := db.Clock.Now()
	insertOrder(t, db, merchantOne, "csv-refunded-one", "refunded one", "refunded", "alipay", now, datePtr(now), datePtr(now), 100)
	insertOrder(t, db, merchantTwo, "csv-refunded-two", "refunded two", "refunded", "wxpay", now.Add(time.Second), datePtr(now.Add(time.Second)), datePtr(now.Add(2*time.Second)), 200)
	insertOrder(t, db, merchantOne, "csv-paid-one", "paid one", "paid", "alipay", now.Add(3*time.Second), datePtr(now.Add(3*time.Second)), nil, 300)
	actors := []platform.Actor{
		{Role: "merchant", SubjectID: merchantOne, MerchantID: merchantOne},
		{Role: "admin", SubjectID: 1},
	}
	t.Run("V48.B.r01/v01", func(t *testing.T) {
		for _, actor := range actors {
			filter := portal.OrderFilter{Status: "refunded", Limit: 100}
			page, err := app.Services.Portal.ListOrders(t.Context(), actor, filter)
			if err != nil {
				t.Fatalf("list refunded orders as %s: %v", actor.Role, err)
			}
			var output bytes.Buffer
			if err := app.Services.Portal.ExportOrders(t.Context(), actor, filter, &output); err != nil {
				t.Fatalf("export refunded orders as %s: %v", actor.Role, err)
			}
			rows, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(output.String(), "\uFEFF"))).ReadAll()
			if err != nil {
				t.Fatalf("decode %s export: %v", actor.Role, err)
			}
			if len(rows) != len(page.Items)+1 {
				t.Fatalf("%s list/export row count differs: list=%d csv=%d", actor.Role, len(page.Items), len(rows)-1)
			}
			want := make(map[string]bool, len(page.Items))
			for _, item := range page.Items {
				want[item.TradeNo] = true
			}
			for _, row := range rows[1:] {
				if !want[row[0]] {
					t.Errorf("%s CSV contains an order outside the filtered list: %q", actor.Role, row[0])
				}
				delete(want, row[0])
			}
			if len(want) != 0 {
				t.Errorf("%s CSV omitted filtered list orders: %#v", actor.Role, want)
			}
		}
	})
	t.Run("V48.B.r01/v02", func(t *testing.T) {
		var output bytes.Buffer
		if err := app.Services.Portal.ExportOrders(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, portal.OrderFilter{Status: "refunded"}, &output); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "csv-refunded-one") || !strings.Contains(output.String(), "csv-refunded-two") || strings.Contains(output.String(), "csv-paid-one") {
			t.Fatalf("refunded order filter export=%q", output.String())
		}
	})
	t.Run("V48.B.r01/v03", func(t *testing.T) {
		var output bytes.Buffer
		if err := app.Services.Portal.ExportOrders(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, portal.OrderFilter{}, &output); err != nil {
			t.Fatal(err)
		}
		header := strings.Split(strings.Split(strings.TrimPrefix(output.String(), "\uFEFF"), "\r\n")[0], ",")
		for _, forbidden := range []string{"receipt_id", "event_id", "raw_text", "payer_hint"} {
			for _, field := range header {
				if strings.EqualFold(field, forbidden) {
					t.Errorf("receipt-only field %q appeared in orders CSV header %#v", forbidden, header)
				}
			}
		}
	})
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, context.DeadlineExceeded }
