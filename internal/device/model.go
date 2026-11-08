package device

import (
	"time"
	"vanillapay/internal/platform"
)

type DeviceDTO struct {
	ID                       *string    `json:"id"`
	Name                     *string    `json:"name"`
	AppVersion               *string    `json:"app_version"`
	Bound                    bool       `json:"bound"`
	Online                   bool       `json:"online"`
	LastSeenAt               *time.Time `json:"last_seen_at"`
	OfflineAt                *time.Time `json:"offline_at"`
	HeartbeatIntervalSeconds int        `json:"heartbeat_interval_seconds"`
	OfflineThresholdSeconds  int        `json:"offline_threshold_seconds"`
}

type BindingDTO struct {
	BindingID string    `json:"binding_id"`
	Code      string    `json:"code"`
	QRContent string    `json:"qr_content"`
	ExpiresAt time.Time `json:"expires_at"`
}
type BindingStatus struct {
	State     string     `json:"state"`
	Device    *DeviceDTO `json:"device"`
	ExpiresAt time.Time  `json:"expires_at"`
}
type RedeemRequest struct {
	Code       string `json:"code"`
	DeviceName string `json:"device_name"`
	AppVersion string `json:"app_version"`
}
type CredentialDTO struct {
	DeviceID                 string    `json:"device_id"`
	DeviceToken              string    `json:"device_token"`
	MerchantID               string    `json:"merchant_id"`
	ServerTime               time.Time `json:"server_time"`
	HeartbeatIntervalSeconds int       `json:"heartbeat_interval_seconds"`
	OfflineThresholdSeconds  int       `json:"offline_threshold_seconds"`
}
type DeviceIdentity = platform.DeviceIdentity
type HeartbeatRequest struct {
	AppVersion string     `json:"app_version"`
	DeviceTime *time.Time `json:"device_time,omitempty"`
}
type DeviceStatus struct {
	ServerTime               time.Time `json:"server_time"`
	Online                   bool      `json:"online"`
	MerchantBanned           bool      `json:"merchant_banned"`
	HeartbeatIntervalSeconds int       `json:"heartbeat_interval_seconds"`
}
type ReceiptRequest struct {
	EventID         string    `json:"event_id"`
	Channel         string    `json:"channel"`
	AmountCents     int64     `json:"amount_cents"`
	OccurredAt      time.Time `json:"occurred_at"`
	ProviderTradeNo string    `json:"provider_trade_no,omitempty"`
	PayerHint       string    `json:"payer_hint,omitempty"`
	RawText         string    `json:"raw_text,omitempty"`
}
type ReceiptResult struct {
	ReceiptID       string    `json:"receipt_id"`
	Duplicate       bool      `json:"duplicate"`
	MatchState      string    `json:"match_state"`
	UnmatchedReason *string   `json:"unmatched_reason"`
	TradeNo         *string   `json:"trade_no"`
	ServerTime      time.Time `json:"server_time"`
}

type CreateBindingRequest struct {
	ReplaceConfirmed bool `json:"replace_confirmed,omitempty"`
}

type UnbindRequest struct {
	Confirm bool `json:"confirm"`
}
