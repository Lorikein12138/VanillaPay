package device

import "time"

const (
	heartbeatIntervalSeconds = 30
	abnormalThreshold        = 90 * time.Second
	offlineThreshold         = 300 * time.Second
	minRecoveryInterval      = 20 * time.Second
	maxRecoveryInterval      = 45 * time.Second
	healthyRecoveryCount     = 3
)

type HealthStatus string

const (
	HealthUnbound  HealthStatus = "unbound"
	HealthOffline  HealthStatus = "offline"
	HealthAbnormal HealthStatus = "abnormal"
	HealthOnline   HealthStatus = "online"
)

func healthStatus(now time.Time, bound bool, lastSeenAt *time.Time, recoveryCount int16) HealthStatus {
	switch {
	case !bound:
		return HealthUnbound
	case lastSeenAt == nil || now.Sub(*lastSeenAt) >= offlineThreshold:
		return HealthOffline
	case lastSeenAt.After(now) || now.Sub(*lastSeenAt) >= abnormalThreshold || recoveryCount < healthyRecoveryCount:
		return HealthAbnormal
	default:
		return HealthOnline
	}
}

type heartbeatState struct {
	lastSeen, lastCounted, onlineSince *time.Time
	everOnline, armed                  bool
	count                              int16
}

func (state heartbeatState) isOffline(now time.Time) bool {
	return state.lastSeen != nil && now.Sub(*state.lastSeen) >= offlineThreshold
}

// Equal timestamps are filtered by the caller. A rollback starts a new run
// anchored at the current server time without changing alert eligibility.
func (state *heartbeatState) record(now time.Time) {
	switch {
	case state.lastSeen != nil && now.Before(*state.lastSeen):
		state.startRecovery(now)
	case !state.everOnline:
		state.count = healthyRecoveryCount
		state.onlineSince = &now
	case state.lastSeen == nil || now.Sub(*state.lastSeen) >= abnormalThreshold:
		state.startRecovery(now)
	case state.count < healthyRecoveryCount:
		switch {
		case state.lastCounted == nil || now.Sub(*state.lastCounted) > maxRecoveryInterval:
			state.startRecovery(now)
		case now.Sub(*state.lastCounted) >= minRecoveryInterval:
			state.count++
			state.lastCounted = &now
		}
	}
	if state.count == healthyRecoveryCount {
		state.lastCounted = nil
		state.armed = true
	}
	state.lastSeen = &now
}

func (state *heartbeatState) startRecovery(now time.Time) {
	state.count = 1
	state.lastCounted = &now
	state.onlineSince = &now
}

func (dto *DeviceDTO) SetHealth(now time.Time, recoveryCount int16) {
	dto.Bound = dto.ID != nil
	dto.HealthStatus = healthStatus(now, dto.Bound, dto.LastSeenAt, recoveryCount)
	dto.Online = dto.HealthStatus == HealthOnline
	dto.HeartbeatIntervalSeconds = heartbeatIntervalSeconds
	dto.AbnormalThresholdSeconds = int(abnormalThreshold.Seconds())
	dto.OfflineThresholdSeconds = int(offlineThreshold.Seconds())
	if dto.LastSeenAt != nil {
		offlineAt := dto.LastSeenAt.Add(offlineThreshold)
		dto.OfflineAt = &offlineAt
	}
}
