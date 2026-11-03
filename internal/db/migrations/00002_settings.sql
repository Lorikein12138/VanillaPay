-- +goose Up
CREATE TABLE media_assets (
    id text PRIMARY KEY,
    owner_merchant_id bigint REFERENCES merchants(id),
    kind text NOT NULL CHECK (kind IN ('qr','logo')),
    channel text CHECK (channel IN ('alipay','wxpay','qqpay')),
    file_path text NOT NULL UNIQUE,
    sha256 bytea NOT NULL,
    mime text NOT NULL CHECK (mime IN ('image/png','image/jpeg')),
    qr_payload_cipher bytea,
    active boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((kind = 'qr' AND owner_merchant_id IS NOT NULL AND channel IS NOT NULL AND qr_payload_cipher IS NOT NULL) OR kind = 'logo')
);
CREATE INDEX media_assets_owner_idx ON media_assets(owner_merchant_id,kind,channel,active);

CREATE TABLE site_settings (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    site jsonb NOT NULL DEFAULT '{"site_name":"VanillaPay","logo_asset_id":null}'::jsonb,
    home jsonb NOT NULL DEFAULT '{"mode":"default","html":null,"url":null}'::jsonb,
    footer_legal jsonb NOT NULL DEFAULT '{"items":[],"terms":{"zh_CN":"","en":""},"privacy":{"zh_CN":"","en":""}}'::jsonb,
    mail jsonb NOT NULL DEFAULT '{"host":"","port":587,"username":"","from_address":"","from_name":"","tls_mode":"starttls"}'::jsonb,
    security jsonb NOT NULL DEFAULT '{"turnstile_site_key":"","allowed_hostname":"","restrict_email_domains":false,"email_domains":[],"ban_contact":{"zh_CN":"","en":""}}'::jsonb,
    payment jsonb NOT NULL DEFAULT '{"min_amount_cents":1,"max_amount_cents":10000000,"default_ttl_seconds":300,"max_ttl_seconds":1800,"float_min_cents":0,"float_max_cents":99,"float_default_cents":10,"float_direction":"down"}'::jsonb,
    billing jsonb NOT NULL DEFAULT '{"merchant_pid":""}'::jsonb,
    updated_at timestamptz NOT NULL DEFAULT now()
);
INSERT INTO site_settings(singleton) VALUES (true);

CREATE TABLE secret_settings (
    name text PRIMARY KEY,
    encrypted_value bytea,
    version bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE legal_versions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('terms','privacy')),
    revision bigint NOT NULL CHECK (revision > 0),
    content jsonb NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(kind,revision)
);
CREATE INDEX legal_versions_current_idx ON legal_versions(kind,revision DESC);

CREATE TABLE legal_acceptances (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id bigint NOT NULL REFERENCES merchants(id),
    kind text NOT NULL CHECK (kind IN ('terms','privacy')),
    revision bigint NOT NULL,
    accepted_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(merchant_id,kind,revision),
    FOREIGN KEY(kind,revision) REFERENCES legal_versions(kind,revision)
);

CREATE TABLE merchant_collection_settings (
    merchant_id bigint PRIMARY KEY REFERENCES merchants(id),
    order_ttl_seconds integer CHECK (order_ttl_seconds IS NULL OR order_ttl_seconds BETWEEN 60 AND 86400),
    float_limit_cents integer CHECK (float_limit_cents IS NULL OR float_limit_cents BETWEEN 0 AND 999),
    float_direction text CHECK (float_direction IS NULL OR float_direction IN ('up','down')),
    version bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE merchant_ip_rules (
    merchant_id bigint PRIMARY KEY REFERENCES merchants(id),
    enabled boolean NOT NULL DEFAULT false,
    entries cidr[] NOT NULL DEFAULT '{}',
    version bigint NOT NULL DEFAULT 1,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (cardinality(entries) <= 100),
    CHECK (NOT enabled OR cardinality(entries) > 0)
);

-- +goose Down
DROP TABLE IF EXISTS merchant_ip_rules;
DROP TABLE IF EXISTS merchant_collection_settings;
DROP TABLE IF EXISTS legal_acceptances;
DROP TABLE IF EXISTS legal_versions;
DROP TABLE IF EXISTS secret_settings;
DROP TABLE IF EXISTS site_settings;
DROP TABLE IF EXISTS media_assets;
