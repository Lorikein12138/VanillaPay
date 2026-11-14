-- +goose Up
ALTER TABLE merchants
    ADD COLUMN approval_state text NOT NULL DEFAULT 'approved'
        CHECK (approval_state IN ('pending','approved','rejected')),
    ADD COLUMN approval_reason text CHECK (char_length(approval_reason) <= 500),
    ADD COLUMN approval_reviewed_at timestamptz,
    ADD COLUMN callback_hide_name boolean NOT NULL DEFAULT false,
    ADD COLUMN callback_alert_armed boolean NOT NULL DEFAULT true,
    ADD COLUMN callback_alert_episode bigint NOT NULL DEFAULT 0;
CREATE INDEX merchants_pending_approval_idx ON merchants(created_at,id) WHERE approval_state='pending';

ALTER TABLE orders ADD COLUMN is_test boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT orders_test_external_check CHECK (NOT is_test OR purpose='external');
CREATE INDEX orders_pending_test_idx ON orders(merchant_id) WHERE is_test AND status='pending';

ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check
        CHECK (kind IN ('refund_request','device_offline','announcement','callback_failed','registration_approved','registration_rejected'));

ALTER TABLE site_settings ALTER COLUMN security SET DEFAULT '{"turnstile_site_key":"","turnstile_enabled":false,"restrict_email_domains":false,"registration_review":false,"email_domains":[],"ban_contact":{"zh_CN":"","en":""}}'::jsonb;
UPDATE site_settings SET security = security || '{"registration_review":false}'::jsonb
    WHERE NOT security ? 'registration_review';

-- +goose Down
ALTER TABLE site_settings ALTER COLUMN security SET DEFAULT '{"turnstile_site_key":"","turnstile_enabled":false,"restrict_email_domains":false,"email_domains":[],"ban_contact":{"zh_CN":"","en":""}}'::jsonb;
UPDATE site_settings SET security = security - 'registration_review';
DELETE FROM notifications WHERE kind IN ('callback_failed','registration_approved','registration_rejected');
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check,
    ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('refund_request','device_offline','announcement'));
DROP INDEX orders_pending_test_idx;
ALTER TABLE orders DROP CONSTRAINT orders_test_external_check, DROP COLUMN is_test;
DROP INDEX merchants_pending_approval_idx;
ALTER TABLE merchants DROP COLUMN approval_state, DROP COLUMN approval_reason, DROP COLUMN approval_reviewed_at,
    DROP COLUMN callback_hide_name, DROP COLUMN callback_alert_armed, DROP COLUMN callback_alert_episode;
