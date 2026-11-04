package merchant

import (
	"testing"
	"time"
)

func TestUnit_ConsoleMerchantCursorIsSignedAndFilterBound(t *testing.T) {
	service := &Service{}
	service.key[0] = 0x5a
	banned := true
	query := ConsoleMerchantQuery{Query: "shop", Banned: &banned, Limit: 20}
	created := time.Date(2026, 10, 31, 8, 30, 0, 123, time.UTC)
	cursor := service.encodeConsoleCursor(created, 17, query)
	gotAt, gotID, err := service.decodeConsoleCursor(cursor, query)
	if err != nil || !gotAt.Equal(created) || gotID != 17 {
		t.Fatalf("cursor decode = %s, %d, %v", gotAt, gotID, err)
	}
	if _, _, err := service.decodeConsoleCursor(cursor, ConsoleMerchantQuery{Query: "other", Banned: &banned}); err == nil {
		t.Fatal("cursor was accepted with a different filter")
	}
}
