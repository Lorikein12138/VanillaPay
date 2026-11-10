-- +goose Up
ALTER TABLE devices
    ADD COLUMN online_since timestamptz,
    ADD COLUMN offline_alert_armed boolean NOT NULL DEFAULT true;

-- A legacy alert followed by brief reconnects still belongs to the same
-- outage under the new rules. Devices online at upgrade start out armed.
UPDATE devices SET offline_alert_armed = false
WHERE revoked_at IS NULL AND last_seen_at <= now() - interval '90 seconds'
    AND offline_notified_episode > 0;

-- +goose Down
UPDATE devices
SET offline_episode = CASE WHEN offline_alert_armed THEN offline_notified_episode
                           ELSE offline_notified_episode - 1 END
WHERE revoked_at IS NULL;

ALTER TABLE devices DROP COLUMN offline_alert_armed, DROP COLUMN online_since;
