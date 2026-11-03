-- +goose Up
CREATE TABLE orders (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    trade_no text NOT NULL UNIQUE,
    merchant_id bigint NOT NULL REFERENCES merchants(id),
    out_trade_no text NOT NULL,
    purpose text NOT NULL CHECK (purpose IN ('external','subscription')),
    protocol text NOT NULL CHECK (protocol IN ('v1','v2','internal')),
    name text NOT NULL,
    product_id text NOT NULL DEFAULT '',
    param text NOT NULL DEFAULT '',
    original_cents bigint NOT NULL CHECK (original_cents BETWEEN 1 AND 999999999),
    actual_cents bigint CHECK (actual_cents BETWEEN 1 AND 999999999),
    amount_offset_cents integer CHECK (amount_offset_cents IS NULL OR amount_offset_cents BETWEEN -999 AND 999),
    settings_version bigint,
    collection_version bigint,
    channel text CHECK (channel IN ('alipay','wxpay','qqpay')),
    allowed_channels_snapshot text[] NOT NULL DEFAULT '{}',
    qr_asset_id text REFERENCES media_assets(id),
    notify_url text,
    return_url text,
    signing_key_version integer,
    signing_key_cipher bytea,
    merchant_username_snapshot text NOT NULL DEFAULT '',
    merchant_pid_snapshot char(16),
    buyer_id bigint REFERENCES merchants(id),
    purchase_id bigint UNIQUE REFERENCES purchases(id),
    expires_at timestamptz NOT NULL,
    channel_locked_at timestamptz,
    status text NOT NULL CHECK (status IN ('pending','paid','expired','refunded')),
    expire_reason text CHECK (expire_reason IS NULL OR expire_reason IN ('timeout','banned','cancelled')),
    paid_at timestamptz,
    refunded_at timestamptz,
    is_manual boolean NOT NULL DEFAULT false,
    manual_reason text,
    manual_actor_id bigint,
    clientip inet,
    buyer_hint text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(merchant_id,out_trade_no),
    CHECK ((channel IS NULL) = (actual_cents IS NULL)),
    CHECK ((status IN ('paid','refunded')) = (paid_at IS NOT NULL)),
    CHECK ((status = 'refunded') = (refunded_at IS NOT NULL)),
    CHECK (status <> 'paid' OR channel IS NOT NULL)
);
CREATE INDEX orders_merchant_created_idx ON orders(merchant_id,created_at DESC,id DESC);
CREATE INDEX orders_pending_expiry_idx ON orders(expires_at,id) WHERE status = 'pending';
CREATE INDEX orders_paid_at_idx ON orders(paid_at) WHERE status IN ('paid','refunded');
CREATE INDEX orders_status_created_idx ON orders(status,created_at DESC,id DESC);

CREATE TABLE amount_reservations (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id bigint NOT NULL REFERENCES merchants(id),
    channel text NOT NULL CHECK (channel IN ('alipay','wxpay','qqpay')),
    actual_cents bigint NOT NULL CHECK (actual_cents BETWEEN 1 AND 999999999),
    order_id bigint NOT NULL UNIQUE REFERENCES orders(id),
    release_after timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(merchant_id,channel,actual_cents)
);
CREATE INDEX amount_reservations_release_idx ON amount_reservations(release_after) WHERE release_after IS NOT NULL;

CREATE TABLE refunds (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE,
    order_id bigint NOT NULL REFERENCES orders(id),
    out_refund_no text,
    requested_cents bigint NOT NULL CHECK (requested_cents > 0),
    status text NOT NULL CHECK (status IN ('pending','rejected','completed')),
    reason text,
    request_digest bytea NOT NULL,
    requested_at timestamptz NOT NULL DEFAULT now(),
    handled_at timestamptz,
    handled_by bigint,
    UNIQUE(order_id,public_id)
);
CREATE UNIQUE INDEX refunds_pending_order_idx ON refunds(order_id) WHERE status = 'pending';
CREATE UNIQUE INDEX refunds_out_no_idx ON refunds(out_refund_no) WHERE out_refund_no IS NOT NULL;
CREATE INDEX refunds_created_idx ON refunds(requested_at DESC,id DESC);

CREATE TABLE callback_events (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE,
    order_id bigint NOT NULL REFERENCES orders(id),
    generation integer NOT NULL CHECK (generation > 0),
    state text NOT NULL CHECK (state IN ('pending','running','success','failed','cancelled','internal_fulfillment')),
    next_attempt_at timestamptz,
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    payload_snapshot jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(order_id,generation)
);
CREATE INDEX callback_events_due_idx ON callback_events(next_attempt_at,id) WHERE state IN ('pending','failed');

CREATE TABLE callback_attempts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE,
    event_id bigint NOT NULL REFERENCES callback_events(id),
    attempt_no integer NOT NULL CHECK (attempt_no > 0),
    started_at timestamptz NOT NULL,
    finished_at timestamptz,
    http_status integer,
    duration_ms bigint,
    response_excerpt text NOT NULL DEFAULT '',
    error_code text,
    next_attempt_at timestamptz,
    url_redacted text NOT NULL DEFAULT '',
    UNIQUE(event_id,attempt_no)
);

-- +goose Down
DROP TABLE IF EXISTS callback_attempts;
DROP TABLE IF EXISTS callback_events;
DROP TABLE IF EXISTS refunds;
DROP TABLE IF EXISTS amount_reservations;
DROP TABLE IF EXISTS orders;
