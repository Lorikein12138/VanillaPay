package payment

import (
	"testing"
	"time"
)

func TestUnit_CallbackRetryPlanAndURLRedaction(t *testing.T) {
	want := []time.Duration{time.Minute, 2 * time.Minute, 5 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour, 6 * time.Hour, 14 * time.Hour}
	for i, delay := range want {
		got, ok := callbackRetryDelay(i + 1)
		if !ok || got != delay {
			t.Errorf("attempt %d delay = %s, retry=%t; want %s", i+1, got, ok, delay)
		}
	}
	if _, ok := callbackRetryDelay(10); ok {
		t.Fatal("callback retried beyond the ten-attempt limit")
	}
	if got := redactCallbackURL("https://example.test/callback?token=secret&order=1"); got != "https://example.test/callback?<redacted>" {
		t.Fatalf("redacted URL = %q", got)
	}
}
