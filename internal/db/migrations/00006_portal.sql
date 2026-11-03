-- +goose Up
CREATE TABLE announcements (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE,
    revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
    title jsonb NOT NULL,
    body jsonb NOT NULL,
    popup boolean NOT NULL DEFAULT false,
    published_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX announcements_visible_idx ON announcements(published_at DESC,id DESC) WHERE deleted_at IS NULL;

CREATE TABLE announcement_reads (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    merchant_id bigint NOT NULL REFERENCES merchants(id),
    announcement_id bigint NOT NULL REFERENCES announcements(id),
    revision bigint NOT NULL,
    read_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(merchant_id,announcement_id,revision)
);
CREATE INDEX announcement_reads_merchant_idx ON announcement_reads(merchant_id,read_at DESC);

CREATE TABLE notifications (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE,
    merchant_id bigint NOT NULL REFERENCES merchants(id),
    kind text NOT NULL CHECK (kind IN ('refund_request','device_offline','announcement')),
    source_id text NOT NULL,
    dedupe_key text NOT NULL UNIQUE,
    safe_payload jsonb NOT NULL DEFAULT '{}'::jsonb,
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX notifications_merchant_created_idx ON notifications(merchant_id,created_at DESC,id DESC);
CREATE INDEX notifications_unread_idx ON notifications(merchant_id,id) WHERE read_at IS NULL;

CREATE TABLE mail_deliveries (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    public_id text NOT NULL UNIQUE,
    dedupe_key text NOT NULL UNIQUE,
    template text NOT NULL,
    recipient_cipher bytea NOT NULL,
    payload_cipher bytea NOT NULL,
    state text NOT NULL CHECK (state IN ('queued','sent','failed')),
    attempts integer NOT NULL DEFAULT 0,
    last_error text,
    sent_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX mail_deliveries_state_idx ON mail_deliveries(state,updated_at);

CREATE TABLE idempotency_records (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_role text NOT NULL,
    actor_id bigint NOT NULL,
    route text NOT NULL,
    key text NOT NULL,
    request_digest bytea NOT NULL,
    result jsonb NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(actor_role,actor_id,route,key)
);
CREATE INDEX idempotency_expiry_idx ON idempotency_records(expires_at);

-- +goose Down
DROP TABLE IF EXISTS idempotency_records;
DROP TABLE IF EXISTS mail_deliveries;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS announcement_reads;
DROP TABLE IF EXISTS announcements;
