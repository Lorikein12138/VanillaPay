-- name: GetBootstrap :one
SELECT singleton, code_hash, expires_at, consumed_at FROM bootstrap WHERE singleton = true;

-- name: ReplaceBootstrapCode :exec
UPDATE bootstrap SET code_hash = $1, expires_at = $2, generated_at = now(), consumed_at = NULL WHERE singleton = true;

-- name: ConsumeBootstrapCode :execrows
UPDATE bootstrap SET consumed_at = now(), code_hash = NULL WHERE singleton = true AND code_hash = $1 AND expires_at > now() AND consumed_at IS NULL;

-- name: GetMerchantByUsername :one
SELECT id, username, username_normalized, email, email_normalized, pid, password_hash, key_cipher, key_version, banned_at, ban_reason, totp_cipher, last_totp_step, preferences, created_at, updated_at FROM merchants WHERE username_normalized = $1;

-- name: GetMerchantByEmail :one
SELECT id, username, username_normalized, email, email_normalized, pid, password_hash, key_cipher, key_version, banned_at, ban_reason, totp_cipher, last_totp_step, preferences, created_at, updated_at FROM merchants WHERE email_normalized = $1;

-- name: InsertMerchant :one
INSERT INTO merchants(username,username_normalized,email,email_normalized,pid,password_hash,key_cipher) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,username,email,pid,created_at;

-- name: InsertSession :one
INSERT INTO sessions(token_hash,csrf_hash,role,subject_id,expires_at,ip,user_agent) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,expires_at;

-- name: RevokeSession :execrows
UPDATE sessions SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: RevokeSubjectSessions :exec
UPDATE sessions SET revoked_at = now() WHERE role = $1 AND subject_id = $2 AND revoked_at IS NULL;

-- name: InsertEmailCode :one
INSERT INTO email_codes(purpose,subject_role,subject_id,email_normalized,target_digest,code_hash,expires_at) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,sent_at,expires_at;

-- name: GetCurrentEmailCode :one
SELECT id,purpose,subject_role,subject_id,email_normalized,target_digest,code_hash,expires_at,sent_at FROM email_codes WHERE purpose=$1 AND subject_role=$2 AND subject_id IS NOT DISTINCT FROM sqlc.narg(subject_id)::bigint AND email_normalized=$3 AND target_digest=$4 AND consumed_at IS NULL ORDER BY sent_at DESC LIMIT 1;
