-- +goose Up
-- +goose StatementBegin
DO $$
DECLARE
    blocked text;
BEGIN
    SELECT string_agg(table_name || '=' || row_count, ', ' ORDER BY table_name) INTO blocked
    FROM (
        SELECT 'orders' AS table_name, count(*) AS row_count FROM orders
            WHERE channel = 'qqpay' OR 'qqpay' = ANY(allowed_channels_snapshot)
        UNION ALL SELECT 'receipts', count(*) FROM receipts WHERE channel = 'qqpay'
        UNION ALL SELECT 'media_assets', count(*) FROM media_assets WHERE channel = 'qqpay'
        UNION ALL SELECT 'amount_reservations', count(*) FROM amount_reservations WHERE channel = 'qqpay'
        UNION ALL SELECT 'plans', count(*) FROM plans WHERE 'qqpay' = ANY(channels)
        UNION ALL SELECT 'subscriptions', count(*) FROM subscriptions WHERE (snapshot->'channels') ? 'qqpay'
        UNION ALL SELECT 'subscription_grants', count(*) FROM subscription_grants WHERE (snapshot->'channels') ? 'qqpay'
        UNION ALL SELECT 'purchases', count(*) FROM purchases WHERE (plan_snapshot->'channels') ? 'qqpay'
    ) AS counts WHERE row_count > 0;
    IF blocked IS NOT NULL THEN
        RAISE EXCEPTION 'Cannot remove qqpay; affected rows by table: %', blocked;
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE orders DROP CONSTRAINT orders_channel_check,
    ADD CONSTRAINT orders_channel_check CHECK (channel IN ('alipay','wxpay')),
    ADD CONSTRAINT orders_allowed_channels_snapshot_check CHECK (allowed_channels_snapshot <@ ARRAY['alipay','wxpay']::text[]),
    ADD COLUMN create_request_digest bytea;
ALTER TABLE receipts DROP CONSTRAINT receipts_channel_check,
    ADD CONSTRAINT receipts_channel_check CHECK (channel IN ('alipay','wxpay'));
ALTER TABLE media_assets DROP CONSTRAINT media_assets_channel_check,
    ADD CONSTRAINT media_assets_channel_check CHECK (channel IN ('alipay','wxpay'));
ALTER TABLE amount_reservations DROP CONSTRAINT amount_reservations_channel_check,
    ADD CONSTRAINT amount_reservations_channel_check CHECK (channel IN ('alipay','wxpay'));
ALTER TABLE plans DROP CONSTRAINT plans_channels_check,
    ADD CONSTRAINT plans_channels_check CHECK (cardinality(channels) > 0 AND channels <@ ARRAY['alipay','wxpay']::text[]);

ALTER TABLE refunds ADD COLUMN merchant_id bigint REFERENCES merchants(id);
UPDATE refunds r SET merchant_id = o.merchant_id FROM orders o WHERE o.id = r.order_id;
ALTER TABLE refunds ALTER COLUMN merchant_id SET NOT NULL,
    ADD CONSTRAINT refunds_merchant_out_no_key UNIQUE (merchant_id, out_refund_no);
DROP INDEX refunds_out_no_idx;

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM refunds WHERE out_refund_no IS NOT NULL GROUP BY out_refund_no HAVING count(*) > 1) THEN
        RAISE EXCEPTION 'Cannot restore refunds_out_no_idx; multiple merchants share an out_refund_no';
    END IF;
END $$;
-- +goose StatementEnd
CREATE UNIQUE INDEX refunds_out_no_idx ON refunds(out_refund_no) WHERE out_refund_no IS NOT NULL;
ALTER TABLE refunds DROP CONSTRAINT refunds_merchant_out_no_key, DROP COLUMN merchant_id;

ALTER TABLE plans DROP CONSTRAINT plans_channels_check,
    ADD CONSTRAINT plans_channels_check CHECK (cardinality(channels) > 0 AND channels <@ ARRAY['alipay','wxpay','qqpay']::text[]);
ALTER TABLE amount_reservations DROP CONSTRAINT amount_reservations_channel_check,
    ADD CONSTRAINT amount_reservations_channel_check CHECK (channel IN ('alipay','wxpay','qqpay'));
ALTER TABLE media_assets DROP CONSTRAINT media_assets_channel_check,
    ADD CONSTRAINT media_assets_channel_check CHECK (channel IN ('alipay','wxpay','qqpay'));
ALTER TABLE receipts DROP CONSTRAINT receipts_channel_check,
    ADD CONSTRAINT receipts_channel_check CHECK (channel IN ('alipay','wxpay','qqpay'));
ALTER TABLE orders DROP CONSTRAINT orders_channel_check, DROP CONSTRAINT orders_allowed_channels_snapshot_check,
    DROP COLUMN create_request_digest,
    ADD CONSTRAINT orders_channel_check CHECK (channel IN ('alipay','wxpay','qqpay'));
