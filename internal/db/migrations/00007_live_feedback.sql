-- +goose Up
ALTER TABLE site_settings
    ALTER COLUMN security SET DEFAULT '{"turnstile_site_key":"","turnstile_enabled":false,"restrict_email_domains":false,"email_domains":[],"ban_contact":{"zh_CN":"","en":""}}'::jsonb,
    ALTER COLUMN site SET DEFAULT '{"site_name":"VanillaPay","logo_asset_id":null,"favicon_asset_id":null}'::jsonb;
-- allowed_hostname is no longer read but is deliberately kept: an image rollback
-- runs the previous binary against this schema, and it still needs the value.
-- The application strips it from reads and carries it over on security saves.
UPDATE site_settings SET security = security || jsonb_build_object('turnstile_enabled',
    COALESCE(btrim(security->>'turnstile_site_key') <> '', false) AND EXISTS (
        SELECT 1 FROM secret_settings WHERE name = 'turnstile_secret' AND octet_length(encrypted_value) > 0
    )), site = site || '{"favicon_asset_id":null}'::jsonb;
ALTER TABLE media_assets DROP CONSTRAINT media_assets_kind_check, DROP CONSTRAINT media_assets_check,
    ADD CONSTRAINT media_assets_kind_check CHECK (kind IN ('qr', 'logo', 'favicon')),
    ADD CONSTRAINT media_assets_check CHECK ((kind = 'qr' AND owner_merchant_id IS NOT NULL AND channel IS NOT NULL AND qr_payload_cipher IS NOT NULL)
        OR kind = 'logo' OR (kind = 'favicon' AND owner_merchant_id IS NULL AND channel IS NULL AND qr_payload_cipher IS NULL));

-- +goose Down
ALTER TABLE site_settings
    ALTER COLUMN security SET DEFAULT '{"turnstile_site_key":"","allowed_hostname":"","restrict_email_domains":false,"email_domains":[],"ban_contact":{"zh_CN":"","en":""}}'::jsonb,
    ALTER COLUMN site SET DEFAULT '{"site_name":"VanillaPay","logo_asset_id":null}'::jsonb;
UPDATE site_settings SET security = (security - 'turnstile_enabled') || jsonb_build_object('allowed_hostname', COALESCE(security->'allowed_hostname', '""'::jsonb)),
    site = site - 'favicon_asset_id';
DELETE FROM media_assets WHERE kind = 'favicon';
ALTER TABLE media_assets DROP CONSTRAINT media_assets_kind_check, DROP CONSTRAINT media_assets_check,
    ADD CONSTRAINT media_assets_kind_check CHECK (kind IN ('qr', 'logo')),
    ADD CONSTRAINT media_assets_check CHECK ((kind = 'qr' AND owner_merchant_id IS NOT NULL AND channel IS NOT NULL AND qr_payload_cipher IS NOT NULL) OR kind = 'logo');
