package contracts

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestHTTP_OrderExportOriginAndFetchMetadata(t *testing.T) {
	fixture, proofs := securityFixture(t)
	for _, role := range []string{"merchant", "admin"} {
		prefix := "merchant"
		if role == "admin" {
			prefix = "console"
		}
		for _, test := range []struct {
			name, origin, site string
			present            bool
			status             int
		}{
			{"valid_origin", "https://pay.example.test", "", true, 200},
			{"valid_origin_wins", "https://pay.example.test", "cross-site", true, 200},
			{"invalid_origin", "https://evil.example", "same-origin", true, 403},
			{"missing_origin_same_origin", "", "same-origin", false, 200},
			{"missing_origin_cross_site", "", "cross-site", false, 403},
			{"missing_origin_same_site", "", "same-site", false, 403},
			{"missing_origin_none", "", "none", false, 403},
			{"missing_both", "", "", false, 403},
			{"empty_origin_not_missing", "", "same-origin", true, 403},
		} {
			t.Run(role+"/"+test.name, func(t *testing.T) {
				proof := proofs[role]
				headers := http.Header{"Cookie": {proof.cookie.Name + "=" + proof.cookie.Value}}
				if test.present {
					headers.Set("Origin", test.origin)
				}
				if test.site != "" {
					headers.Set("Sec-Fetch-Site", test.site)
				}
				response := fixture.RequestWithHeaders(t, http.MethodGet, "/api/v1/"+prefix+"/orders/export.csv", nil, "guest", headers)
				if response.Code != test.status {
					t.Fatalf("export status=%d want=%d", response.Code, test.status)
				}
				if test.status == http.StatusOK {
					if !strings.HasPrefix(response.Header().Get("Content-Type"), "text/csv") || !strings.Contains(response.Body.String(), "trade_no") {
						t.Fatal("export did not reach the CSV handler")
					}
				} else {
					var envelope struct {
						Error struct {
							Code string `json:"code"`
						} `json:"error"`
					}
					if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Error.Code != "CSRF_FAILED" {
						t.Fatal("export did not return CSRF_FAILED")
					}
				}
			})
		}
	}
}
