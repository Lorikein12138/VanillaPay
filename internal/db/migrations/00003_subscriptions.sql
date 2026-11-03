-- +goose Up
CREATE TABLE plans (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    name_zh text NOT NULL,
    name_en text NOT NULL DEFAULT '',
    price_cents bigint NOT NULL CHECK (price_cents BETWEEN 1 AND 999999999),
    duration_days integer NOT NULL CHECK (duration_days BETWEEN 1 AND 36500),
    channels text[] NOT NULL CHECK (cardinality(channels) > 0 AND channels <@ ARRAY['alipay','wxpay','qqpay']::text[]),
    purchase_limit integer CHECK (purchase_limit IS NULL OR purchase_limit > 0),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX plans_visible_idx ON plans(id) WHERE deleted_at IS NULL;

CREATE TABLE subscriptions (
    merchant_id bigint PRIMARY KEY REFERENCES merchants(id),
    plan_id bigint REFERENCES plans(id),
    snapshot jsonb,
    expires_at timestamptz,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    active_chain_id text,
    source text NOT NULL DEFAULT 'none' CHECK (source IN ('purchase','manual','none')),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX subscriptions_expiry_idx ON subscriptions(expires_at) WHERE plan_id IS NOT NULL;

CREATE TABLE subscription_grants (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    purchase_id bigint UNIQUE,
    merchant_id bigint NOT NULL REFERENCES merchants(id),
    chain_id text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('new','renew','switch','manual')),
    duration interval,
    effective_at timestamptz NOT NULL,
    fixed_expires_at timestamptz,
    snapshot jsonb NOT NULL,
    voided_at timestamptz,
    refunded_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((kind = 'manual') OR duration IS NOT NULL)
);
CREATE INDEX subscription_grants_chain_idx ON subscription_grants(merchant_id,chain_id,effective_at,id);

CREATE TABLE purchases (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE,
    buyer_id bigint NOT NULL REFERENCES merchants(id),
    billing_merchant_id bigint NOT NULL REFERENCES merchants(id),
    plan_id bigint NOT NULL REFERENCES plans(id),
    plan_snapshot jsonb NOT NULL,
    purchase_kind text NOT NULL CHECK (purchase_kind IN ('new','renew','switch')),
    expected_subscription_revision bigint NOT NULL,
    state text NOT NULL CHECK (state IN ('pending','paid','expired','cancelled','refunded')),
    payment_trade_no text UNIQUE,
    idempotency_key text NOT NULL,
    request_digest bytea NOT NULL,
    fulfillment_at timestamptz,
    paid_at timestamptz,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (state <> 'paid' OR (paid_at IS NOT NULL AND fulfillment_at IS NOT NULL))
);
CREATE UNIQUE INDEX purchases_one_pending_buyer_idx ON purchases(buyer_id) WHERE state = 'pending';
CREATE UNIQUE INDEX purchases_idempotency_idx ON purchases(buyer_id,idempotency_key);
CREATE INDEX purchases_created_idx ON purchases(created_at DESC,id DESC);
CREATE INDEX purchases_state_expiry_idx ON purchases(state,expires_at);

CREATE TABLE plan_purchase_counters (
    merchant_id bigint NOT NULL REFERENCES merchants(id),
    plan_id bigint NOT NULL REFERENCES plans(id),
    paid_count integer NOT NULL DEFAULT 0 CHECK (paid_count >= 0),
    reserved_count integer NOT NULL DEFAULT 0 CHECK (reserved_count >= 0),
    PRIMARY KEY(merchant_id,plan_id)
);
ALTER TABLE subscription_grants ADD CONSTRAINT subscription_grants_purchase_fk FOREIGN KEY (purchase_id) REFERENCES purchases(id);

-- +goose Down
DROP TABLE IF EXISTS plan_purchase_counters;
DROP TABLE IF EXISTS subscription_grants;
DROP TABLE IF EXISTS purchases;
DROP TABLE IF EXISTS subscriptions;
DROP TABLE IF EXISTS plans;
