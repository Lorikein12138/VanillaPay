-- +goose Up
ALTER TABLE plans
    ADD COLUMN min_order_cents bigint CHECK (min_order_cents BETWEEN 1 AND 999999999),
    ADD COLUMN max_order_cents bigint CHECK (max_order_cents BETWEEN 1 AND 999999999),
    ADD COLUMN daily_limit_cents bigint CHECK (daily_limit_cents BETWEEN 1 AND 999999999),
    ADD CONSTRAINT plans_order_bounds_check CHECK (min_order_cents <= max_order_cents);

ALTER TABLE site_settings ADD COLUMN risk jsonb NOT NULL DEFAULT
    '{"ip_daily_order_limit":0,"ip_blacklist":[],"name_block_words":[],"checkout_turnstile_enabled":false}';

CREATE TABLE merchant_notify_domains (
    merchant_id bigint PRIMARY KEY REFERENCES merchants(id),
    enabled boolean NOT NULL DEFAULT false,
    entries text[] NOT NULL DEFAULT '{}',
    version bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (cardinality(entries) <= 20),
    CHECK (NOT enabled OR cardinality(entries) > 0)
);

CREATE INDEX orders_external_ip_created_idx ON orders(clientip,created_at)
    WHERE purpose='external' AND clientip IS NOT NULL;
CREATE INDEX orders_external_merchant_paid_idx ON orders(merchant_id,paid_at) INCLUDE (original_cents)
    WHERE purpose='external' AND status='paid';
CREATE INDEX orders_external_merchant_pending_idx ON orders(merchant_id,expires_at) INCLUDE (original_cents)
    WHERE purpose='external' AND status='pending';

-- +goose Down
DROP INDEX orders_external_merchant_pending_idx;
DROP INDEX orders_external_merchant_paid_idx;
DROP INDEX orders_external_ip_created_idx;
DROP TABLE merchant_notify_domains;
ALTER TABLE site_settings DROP COLUMN risk;
ALTER TABLE plans DROP CONSTRAINT plans_order_bounds_check,
    DROP COLUMN min_order_cents, DROP COLUMN max_order_cents, DROP COLUMN daily_limit_cents;
