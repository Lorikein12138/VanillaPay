-- +goose Up
CREATE TABLE devices (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE,
    merchant_id bigint NOT NULL REFERENCES merchants(id),
    token_hash bytea NOT NULL UNIQUE,
    name text NOT NULL,
    app_version text NOT NULL DEFAULT '',
    last_seen_at timestamptz,
    ever_online boolean NOT NULL DEFAULT false,
    offline_episode integer NOT NULL DEFAULT 0,
    offline_notified_episode integer NOT NULL DEFAULT 0,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX devices_one_active_per_merchant_idx ON devices(merchant_id) WHERE revoked_at IS NULL;
CREATE INDEX devices_last_seen_idx ON devices(last_seen_at) WHERE revoked_at IS NULL;

CREATE TABLE device_bindings (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE,
    code_hash bytea NOT NULL UNIQUE,
    merchant_id bigint NOT NULL REFERENCES merchants(id),
    replace_confirmed boolean NOT NULL DEFAULT false,
    device_name text NOT NULL DEFAULT '',
    app_version text NOT NULL DEFAULT '',
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    device_id bigint REFERENCES devices(id),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX device_bindings_active_idx ON device_bindings(merchant_id,expires_at) WHERE consumed_at IS NULL;

CREATE TABLE receipts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE,
    merchant_id bigint NOT NULL REFERENCES merchants(id),
    device_id bigint NOT NULL REFERENCES devices(id),
    event_id uuid NOT NULL,
    channel text NOT NULL CHECK (channel IN ('alipay','wxpay','qqpay')),
    amount_cents bigint NOT NULL CHECK (amount_cents BETWEEN 1 AND 999999999),
    occurred_at timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now(),
    provider_trade_no text,
    payer_hint text,
    raw_text_cipher bytea,
    raw_text_hash bytea,
    match_state text NOT NULL CHECK (match_state IN ('matched','unmatched')),
    unmatched_reason text,
    order_id bigint UNIQUE REFERENCES orders(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(merchant_id,event_id),
    CHECK ((match_state = 'matched') = (order_id IS NOT NULL)),
    CHECK ((match_state = 'unmatched') = (unmatched_reason IS NOT NULL))
);
CREATE UNIQUE INDEX receipts_provider_trade_idx ON receipts(merchant_id,channel,provider_trade_no) WHERE provider_trade_no IS NOT NULL;
CREATE INDEX receipts_merchant_created_idx ON receipts(merchant_id,received_at DESC,id DESC);
CREATE INDEX receipts_match_created_idx ON receipts(merchant_id,match_state,received_at DESC,id DESC);

-- +goose Down
DROP TABLE IF EXISTS receipts;
DROP TABLE IF EXISTS device_bindings;
DROP TABLE IF EXISTS devices;
