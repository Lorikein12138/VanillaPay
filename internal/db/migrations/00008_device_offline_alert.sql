-- +goose Up
ALTER TABLE devices
    ADD COLUMN online_since timestamptz,
    ADD COLUMN offline_alert_armed boolean NOT NULL DEFAULT true;

-- Legacy counters do not distinguish stable recovery from brief reconnects.
UPDATE devices
SET offline_alert_armed = false,
    online_since = CASE WHEN last_seen_at > now() - interval '90 seconds' THEN now() END
WHERE offline_notified_episode > 0;

ALTER TABLE mail_deliveries
    ADD COLUMN lease_token uuid,
    ADD COLUMN lease_expires_at timestamptz,
    ADD COLUMN next_attempt_at timestamptz,
    ADD CONSTRAINT mail_deliveries_lease_pair CHECK ((lease_token IS NULL) = (lease_expires_at IS NULL));

UPDATE mail_deliveries
SET next_attempt_at = updated_at + CASE attempts WHEN 1 THEN interval '1 minute'
    WHEN 2 THEN interval '5 minutes' WHEN 3 THEN interval '30 minutes' END
WHERE state IN ('queued','failed') AND attempts BETWEEN 1 AND 3;

UPDATE mail_deliveries SET state = 'failed', last_error = 'smtp_delivery_failed'
WHERE state = 'queued' AND attempts >= 4;

-- +goose Down
ALTER TABLE mail_deliveries
    DROP CONSTRAINT mail_deliveries_lease_pair,
    DROP COLUMN lease_token, DROP COLUMN lease_expires_at, DROP COLUMN next_attempt_at;

UPDATE devices
SET offline_episode = CASE WHEN offline_alert_armed THEN offline_notified_episode
                           ELSE offline_notified_episode - 1 END
WHERE revoked_at IS NULL;

ALTER TABLE devices DROP COLUMN offline_alert_armed, DROP COLUMN online_since;
