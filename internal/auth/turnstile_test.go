package auth

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type turnstileRoundTrip func(*http.Request) (*http.Response, error)

func (f turnstileRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUnit_SiteverifyRequiresSuccessActionAndHostname(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		want    bool
		wantErr bool
	}{
		{"valid", `{"success":true,"action":"login","hostname":"pay.example.com"}`, true, false},
		{"wrong-action", `{"success":true,"action":"register","hostname":"pay.example.com"}`, false, false},
		{"wrong-host", `{"success":true,"action":"login","hostname":"evil.example.com"}`, false, false},
		{"bad-json", `{`, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := &http.Client{Transport: turnstileRoundTrip(func(req *http.Request) (*http.Response, error) {
				if req.URL.Host != "challenges.cloudflare.com" || req.Method != http.MethodPost {
					t.Fatalf("unexpected Siteverify request: %s %s", req.Method, req.URL)
				}
				body, err := io.ReadAll(req.Body)
				if err != nil || !strings.Contains(string(body), "response=token") || !strings.Contains(string(body), "secret=secret") {
					t.Fatalf("Siteverify form body = %q, err=%v", body, err)
				}
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(tc.body)), Header: make(http.Header)}, nil
			})}
			got, err := (Siteverify{Client: client}).Verify(context.Background(), "secret", "token", "login", "pay.example.com")
			if got != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("Verify() = %t, %v; want %t, error=%t", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
