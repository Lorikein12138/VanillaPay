package portal

import (
	"testing"
	"time"
)

func TestUnit_StatsQueryDefaultsTimezoneAndAcceptsHalfOpenRange(t *testing.T) {
	start := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	end := start.Add(366 * 24 * time.Hour)
	got, err := normalizeStatsQuery(StatsQuery{Start: start, End: end})
	if err != nil {
		t.Fatalf("normalizeStatsQuery() rejected 366-day range: %v", err)
	}
	if got.Timezone != "Asia/Shanghai" {
		t.Fatalf("default timezone = %q", got.Timezone)
	}
	if !got.Start.Equal(start) || !got.End.Equal(end) {
		t.Fatalf("normalized boundaries = %s .. %s", got.Start, got.End)
	}
}

func TestUnit_StatsQueryRejectsInvalidTimezoneAndRange(t *testing.T) {
	start := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	for name, query := range map[string]StatsQuery{
		"unknown timezone": {Start: start, End: start.Add(time.Hour), Timezone: "Mars/Olympus"},
		"empty range":      {Start: start, End: start},
		"reversed range":   {Start: start.Add(time.Hour), End: start},
		"over 366 days":    {Start: start, End: start.Add(366*24*time.Hour + time.Nanosecond)},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeStatsQuery(query); err == nil {
				t.Fatal("normalizeStatsQuery() accepted invalid query")
			}
		})
	}
}
