package reporting

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"strings"
	"testing"
	"time"

	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
	"vanillapay/tests/backend/testsupport"
)

func TestDB_ReportingRemainingAcceptance(t *testing.T) {
	db := testsupport.NewDatabase(t)
	merchantID, _ := db.SeedMerchant(t)
	app := db.NewApplication(t)
	actor := platform.Actor{Role: "merchant", SubjectID: merchantID, MerchantID: merchantID}
	end := db.Clock.Now().UTC().Truncate(time.Microsecond)
	start := end.Add(-24 * time.Hour)
	insertOrder(t, db, merchantID, "acceptance-report-paid-start", "paid at start", "paid", "alipay", start, datePtr(start), nil, 125)
	insertOrder(t, db, merchantID, "acceptance-report-pending", "pending", "pending", "", start.Add(time.Hour), nil, nil, 75)
	insideEnd := end.Add(-time.Second)
	insertOrder(t, db, merchantID, "acceptance-report-paid-end", "paid before end", "paid", "wxpay", insideEnd, datePtr(insideEnd), nil, 225)
	insertOrder(t, db, merchantID, "acceptance-report-expired", "expired", "expired", "qqpay", insideEnd, nil, nil, 50)
	insertOrder(t, db, merchantID, "acceptance-report-at-end", "excluded at end", "paid", "qqpay", end, datePtr(end), nil, 900)

	t.Run("V46.B.r02/v01", func(t *testing.T) {
		stats, err := app.Services.Portal.Stats(t.Context(), actor, portal.StatsQuery{Start: start, End: end, Timezone: "UTC"})
		if err != nil {
			t.Fatalf("query half-open reporting range: %v", err)
		}
		if stats.TotalOrderCount != 4 || stats.StatusCounts["paid"] != 2 || stats.StatusCounts["pending"] != 1 || stats.StatusCounts["expired"] != 1 {
			t.Fatalf("left-inclusive/right-exclusive order counts=%d status=%#v", stats.TotalOrderCount, stats.StatusCounts)
		}
		if stats.TotalAmountCents != 350 || stats.Channels["alipay"].AmountCents != 125 || stats.Channels["wxpay"].AmountCents != 225 || stats.AmountBasis != "paid_at" || stats.OrderBasis != "created_at" {
			t.Fatalf("paid-at half-open amounts=%d channels=%#v bases=%s/%s", stats.TotalAmountCents, stats.Channels, stats.AmountBasis, stats.OrderBasis)
		}
	})
	t.Run("V46.B.r02/v02", func(t *testing.T) {
		for _, sample := range []struct {
			days       int
			wantAmount int64
			wantOrders int64
		}{
			{days: 1, wantAmount: 350, wantOrders: 4},
			{days: 7, wantAmount: 350, wantOrders: 4},
			{days: 30, wantAmount: 350, wantOrders: 4},
		} {
			startRange := end.AddDate(0, 0, -sample.days)
			shortcut, err := app.Services.Portal.Stats(t.Context(), actor, portal.StatsQuery{Start: startRange, End: end, Timezone: "UTC"})
			if err != nil {
				t.Fatalf("query %d-day shortcut interval: %v", sample.days, err)
			}
			customRange := portal.StatsQuery{Start: end.Add(-time.Duration(sample.days) * 24 * time.Hour), End: end, Timezone: "UTC"}
			custom, err := app.Services.Portal.Stats(t.Context(), actor, customRange)
			if err != nil || shortcut.TotalAmountCents != custom.TotalAmountCents || shortcut.TotalOrderCount != custom.TotalOrderCount || len(shortcut.Trend) != len(custom.Trend) {
				t.Fatalf("%d-day shortcut does not match explicit interval: shortcut=%#v custom=%#v err=%v", sample.days, shortcut, custom, err)
			}
			if shortcut.TotalAmountCents != sample.wantAmount || shortcut.TotalOrderCount != sample.wantOrders {
				t.Fatalf("%d-day range total=%d orders=%d, want %d/%d", sample.days, shortcut.TotalAmountCents, shortcut.TotalOrderCount, sample.wantAmount, sample.wantOrders)
			}
		}
	})
	t.Run("V46.B.r02/v03", func(t *testing.T) {
		empty, err := app.Services.Portal.Stats(t.Context(), actor, portal.StatsQuery{Start: end.AddDate(0, 0, -7), End: end.AddDate(0, 0, -6), Timezone: "UTC"})
		if err != nil {
			t.Fatalf("query empty interval: %v", err)
		}
		if empty.TotalAmountCents != 0 || empty.TotalOrderCount != 0 || empty.StatusCounts["paid"] != 0 || empty.Channels["alipay"].AmountCents != 0 || len(empty.Trend) != 2 || empty.Trend[0].AmountCents != 0 || empty.Trend[1].AmountCents != 0 {
			t.Fatalf("empty statistics not zero-filled: %#v", empty)
		}
	})

	t.Run("V47.B.r04/v01", func(t *testing.T) {
		if _, err := app.Services.Portal.Stats(t.Context(), actor, portal.StatsQuery{Start: end.AddDate(0, 0, -367), End: end, Timezone: "UTC"}); err == nil {
			t.Fatal("statistics query longer than the 366-day hard limit was accepted")
		}
		if _, err := app.Services.Portal.Stats(t.Context(), actor, portal.StatsQuery{Start: end, End: start, Timezone: "UTC"}); err == nil {
			t.Fatal("inverted statistics range was accepted")
		}
	})

	t.Run("V48.B.r03/v01", func(t *testing.T) {
		var output bytes.Buffer
		if err := app.Services.Portal.ExportOrders(t.Context(), actor, portal.OrderFilter{Status: "paid"}, &output); err != nil {
			t.Fatalf("export all pages of filtered paid orders: %v", err)
		}
		reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(output.String(), "\uFEFF")))
		rows, err := reader.ReadAll()
		if err != nil {
			t.Fatalf("decode exported CSV: %v", err)
		}
		if len(rows) != 4 || rows[1][0] != "acceptance-report-at-end" || rows[2][0] != "acceptance-report-paid-end" || rows[3][0] != "acceptance-report-paid-start" {
			t.Fatalf("export CSV did not include the full stable filtered set: %#v", rows)
		}
		for _, sensitive := range []string{"merchant_key", "clientip", "signing_key_cipher", "private_key"} {
			if strings.Contains(strings.ToLower(output.String()), sensitive) {
				t.Errorf("CSV output exposed sensitive field %q", sensitive)
			}
		}
	})
	t.Run("V48.B.r03/v02", func(t *testing.T) {
		for index := 0; index < 125; index++ {
			at := start.Add(time.Duration(index) * time.Second)
			insertOrder(t, db, merchantID, fmt.Sprintf("acceptance-report-page-%03d", index), "paged export", "paid", "alipay", at, datePtr(at), nil, 25)
		}
		var output bytes.Buffer
		if err := app.Services.Portal.ExportOrders(t.Context(), actor, portal.OrderFilter{Status: "paid"}, &output); err != nil {
			t.Fatalf("export beyond one list page: %v", err)
		}
		reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(output.String(), "\uFEFF")))
		rows, err := reader.ReadAll()
		if err != nil || len(rows) != 129 {
			t.Fatalf("full export rows=%d err=%v, want header plus all 128 paid orders", len(rows), err)
		}
	})
	t.Run("V48.B.r03/v03", func(t *testing.T) {
		var output bytes.Buffer
		if err := app.Services.Portal.ExportOrders(t.Context(), actor, portal.OrderFilter{Status: "paid"}, &output); err != nil {
			t.Fatalf("export orders for sensitive field check: %v", err)
		}
		for _, sensitive := range []string{"merchant_key", "clientip", "signing_key_cipher", "private_key", "192.0.2."} {
			if strings.Contains(strings.ToLower(output.String()), strings.ToLower(sensitive)) {
				t.Errorf("CSV exposed sensitive value %q", sensitive)
			}
		}
	})

}
