-- +goose Up
ALTER TABLE devices
    ADD COLUMN recovery_count smallint NOT NULL DEFAULT 0,
    ADD COLUMN last_counted_heartbeat_at timestamptz,
    ADD CONSTRAINT devices_heartbeat_recovery_check CHECK (
        recovery_count BETWEEN 0 AND 3
        AND (last_counted_heartbeat_at IS NOT NULL) = (recovery_count IN (1, 2))
    );

-- Carry forward the old effective arming rule before the worker stops using it.
UPDATE devices
SET offline_alert_armed = true,
    recovery_count = CASE WHEN last_seen_at > now() - interval '90 seconds'
                              AND last_seen_at <= now() THEN 3 ELSE 0 END
WHERE revoked_at IS NULL AND ever_online AND last_seen_at IS NOT NULL
    AND (offline_alert_armed OR last_seen_at > online_since + interval '210 seconds');

-- +goose Down
ALTER TABLE devices
    DROP CONSTRAINT devices_heartbeat_recovery_check,
    DROP COLUMN last_counted_heartbeat_at,
    DROP COLUMN recovery_count;
