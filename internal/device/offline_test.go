package device

import (
	"testing"
)

func TestUnit_OfflineNotificationEpisodesAreMonotonic(t *testing.T) {
	for _, tc := range []struct {
		episode  int64
		notified int64
		want     bool
	}{{0, 0, true}, {1, 0, true}, {1, 1, true}, {1, 2, false}, {4, 5, false}} {
		if got := needsOfflineNotification(tc.episode, tc.notified); got != tc.want {
			t.Errorf("needsOfflineNotification(%d,%d) = %t, want %t", tc.episode, tc.notified, got, tc.want)
		}
	}
}
