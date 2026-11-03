-- +goose Up
CREATE TABLE admins (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY CHECK (id = 1),
    username text NOT NULL,
    username_normalized text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    totp_cipher bytea,
    last_totp_step bigint NOT NULL DEFAULT -1,
    preferences jsonb NOT NULL DEFAULT '{"locale":"zh-CN","theme":"system"}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE bootstrap (
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    code_hash bytea,
    expires_at timestamptz,
    generated_at timestamptz,
    consumed_at timestamptz
);
INSERT INTO bootstrap(singleton) VALUES (true);

CREATE TABLE merchants (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    username text NOT NULL,
    username_normalized text NOT NULL UNIQUE,
    email text NOT NULL,
    email_normalized text NOT NULL UNIQUE,
    pid char(16) NOT NULL UNIQUE CHECK (pid ~ '^[1-9][0-9]{15}$'),
    password_hash text NOT NULL,
    key_cipher bytea NOT NULL,
    key_version integer NOT NULL DEFAULT 1 CHECK (key_version > 0),
    rsa_public_key_pem text,
    rsa_key_updated_at timestamptz,
    banned_at timestamptz,
    ban_reason text,
    totp_cipher bytea,
    last_totp_step bigint NOT NULL DEFAULT -1,
    preferences jsonb NOT NULL DEFAULT '{"locale":"zh-CN","theme":"system"}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((banned_at IS NULL) = (ban_reason IS NULL))
);

CREATE TABLE sessions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    token_hash bytea NOT NULL UNIQUE,
    csrf_hash bytea NOT NULL,
    role text NOT NULL CHECK (role IN ('merchant','admin')),
    subject_id bigint NOT NULL,
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    ip inet,
    user_agent text NOT NULL DEFAULT ''
);
CREATE INDEX sessions_subject_active_idx ON sessions(role, subject_id, expires_at) WHERE revoked_at IS NULL;

CREATE TABLE email_codes (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    purpose text NOT NULL CHECK (purpose IN ('register','reset_password','change_username','change_email_old','change_email_new','change_password','reset_key')),
    subject_role text NOT NULL CHECK (subject_role IN ('merchant','admin','public')),
    subject_id bigint,
    email_normalized text NOT NULL,
    target_digest bytea NOT NULL,
    code_hash bytea NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
    sent_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX email_codes_recent_idx ON email_codes(purpose,email_normalized,sent_at DESC);
CREATE INDEX email_codes_subject_idx ON email_codes(subject_role,subject_id,purpose,sent_at DESC);

CREATE TABLE auth_challenges (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    challenge_hash bytea NOT NULL UNIQUE,
    role text NOT NULL CHECK (role IN ('merchant','admin')),
    subject_id bigint NOT NULL,
    purpose text NOT NULL DEFAULT 'login' CHECK (purpose IN ('login','totp_setup','recovery')),
    secret_cipher bytea,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    failed_attempts integer NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE recovery_codes (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    role text NOT NULL CHECK (role IN ('merchant','admin')),
    subject_id bigint NOT NULL,
    code_hash bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    used_at timestamptz
);
CREATE INDEX recovery_codes_subject_idx ON recovery_codes(role,subject_id) WHERE used_at IS NULL;

CREATE TABLE audit_logs (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_role text NOT NULL CHECK (actor_role IN ('merchant','admin','system','device')),
    actor_id bigint,
    merchant_id bigint REFERENCES merchants(id),
    action text NOT NULL,
    target_type text NOT NULL,
    target_id text NOT NULL,
    result text NOT NULL CHECK (result IN ('success','failure')),
    ip inet,
    request_id text NOT NULL,
    safe_metadata jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_merchant_created_idx ON audit_logs(merchant_id,created_at DESC,id DESC);
CREATE INDEX audit_logs_actor_created_idx ON audit_logs(actor_role,actor_id,created_at DESC,id DESC);

-- +goose Down
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS recovery_codes;
DROP TABLE IF EXISTS auth_challenges;
DROP TABLE IF EXISTS email_codes;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS merchants;
DROP TABLE IF EXISTS bootstrap;
DROP TABLE IF EXISTS admins;
