-- name: GetSiteSettings :one
SELECT version,site,home,footer_legal,mail,security,payment,billing,updated_at FROM site_settings WHERE singleton=true;

-- name: GetSecretSetting :one
SELECT name,encrypted_value,version,updated_at FROM secret_settings WHERE name=$1;

-- name: PutSecretSetting :exec
INSERT INTO secret_settings(name,encrypted_value,version,updated_at) VALUES($1,$2,1,now()) ON CONFLICT(name) DO UPDATE SET encrypted_value=EXCLUDED.encrypted_value,version=secret_settings.version+1,updated_at=now();

-- name: UpdateSettingGroup :one
UPDATE site_settings SET
  site=CASE WHEN $2='site' THEN $3::jsonb ELSE site END,
  home=CASE WHEN $2='home' THEN $3::jsonb ELSE home END,
  footer_legal=CASE WHEN $2='footer_legal' THEN $3::jsonb ELSE footer_legal END,
  mail=CASE WHEN $2='mail' THEN $3::jsonb ELSE mail END,
  security=CASE WHEN $2='security' THEN $3::jsonb ELSE security END,
  payment=CASE WHEN $2='payment' THEN $3::jsonb ELSE payment END,
  billing=CASE WHEN $2='billing' THEN $3::jsonb ELSE billing END,
  version=version+1,updated_at=now()
WHERE singleton=true AND version=$1 RETURNING version,site,home,footer_legal,mail,security,payment,billing,updated_at;

-- name: GetCollectionSettings :one
SELECT merchant_id,order_ttl_seconds,float_limit_cents,float_direction,version,updated_at FROM merchant_collection_settings WHERE merchant_id=$1;

-- name: UpsertCollectionSettings :one
INSERT INTO merchant_collection_settings(merchant_id,order_ttl_seconds,float_limit_cents,float_direction) VALUES($1,$2,$3,$4)
ON CONFLICT(merchant_id) DO UPDATE SET order_ttl_seconds=EXCLUDED.order_ttl_seconds,float_limit_cents=EXCLUDED.float_limit_cents,float_direction=EXCLUDED.float_direction,version=merchant_collection_settings.version+1,updated_at=now()
RETURNING merchant_id,order_ttl_seconds,float_limit_cents,float_direction,version,updated_at;

-- name: GetMerchantQRs :many
SELECT id,channel,file_path,mime,active,created_at FROM media_assets WHERE owner_merchant_id=$1 AND kind='qr' ORDER BY channel;
