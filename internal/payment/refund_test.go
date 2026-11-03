package payment

import (
	"strings"
	"testing"

	"vanillapay/internal/platform"
)

func TestUnit_RefundRequestValidatesActorAndFields(t *testing.T) {
	valid := RefundCommand{MerchantID: 7, TradeNo: "trade-1", AmountCents: 100, Actor: platform.Actor{Role: "merchant", MerchantID: 7}}
	if err := validateRefundRequest(valid); err != nil {
		t.Fatalf("valid refund request rejected: %v", err)
	}
	for _, input := range []RefundCommand{
		{MerchantID: 7, TradeNo: "trade-1", AmountCents: 100, Actor: platform.Actor{Role: "merchant", MerchantID: 8}},
		{MerchantID: 7, TradeNo: " ", AmountCents: 100, Actor: platform.Actor{Role: "merchant", MerchantID: 7}},
		{MerchantID: 7, TradeNo: "trade-1", AmountCents: 0, Actor: platform.Actor{Role: "merchant", MerchantID: 7}},
		{MerchantID: 7, TradeNo: "trade-1", AmountCents: 100, OutRefundNo: strings.Repeat("x", 65), Actor: platform.Actor{Role: "merchant", MerchantID: 7}},
	} {
		if err := validateRefundRequest(input); err == nil {
			t.Errorf("invalid refund request unexpectedly accepted: %#v", input)
		}
	}
}
