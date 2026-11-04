package contracts

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"vanillapay/internal/auth"
	"vanillapay/internal/device"
	"vanillapay/internal/platform"
	"vanillapay/internal/settings"
	"vanillapay/tests/backend/testsupport"
)

type sessionProof struct {
	cookie *http.Cookie
	csrf   string
}

func TestHTTP_OpenAPISecurityMatrix(t *testing.T) {
	fixture, proofs := securityFixture(t)
	doc := testsupport.OpenAPI(t)
	cases := 0
	for path, item := range doc.Paths.Map() {
		for method, operation := range item.Operations() {
			body := []byte(`{}`)
			conditional := false
			if operation.RequestBody != nil {
				if media := operation.RequestBody.Value.Content.Get("application/json"); media != nil {
					if example := media.Examples["authenticated"]; example != nil {
						var err error
						body, err = json.Marshal(example.Value.Value)
						if err != nil {
							t.Fatal(err)
						}
						conditional = true
					}
				}
			}
			public, requiredRole, csrf, deviceOnly, protocol := false, "", false, false, false
			for _, requirement := range *operation.Security {
				if len(requirement) == 0 {
					public = true
				}
				for scheme := range requirement {
					switch scheme {
					case "MerchantSession":
						requiredRole = "merchant"
					case "AdminSession":
						requiredRole = "admin"
					case "CSRF":
						csrf = true
					case "DeviceToken":
						deviceOnly = true
					case "PaymentV1Signature", "PaymentV2Signature", "PaymentV1Key":
						protocol = true
					}
				}
			}
			if public && !conditional {
				continue
			}
			if requiredRole == "" && !deviceOnly && !protocol {
				continue
			}
			// IDs need only be syntactically valid: authorization precedes the
			// handler/resource lookup, so denied requests cannot mutate data.
			target := regexp.MustCompile(`\{[^}]+\}`).ReplaceAllString(path, "1")
			testCases := []struct {
				name, role, token string
				status            int
				code              string
			}{}
			if requiredRole != "" {
				opposite := "merchant"
				if requiredRole == "merchant" {
					opposite = "admin"
				}
				testCases = append(testCases,
					struct {
						name, role, token string
						status            int
						code              string
					}{"guest", "", "", 401, "UNAUTHENTICATED"},
					struct {
						name, role, token string
						status            int
						code              string
					}{"opposite_session", opposite, "", 403, "FORBIDDEN"},
					struct {
						name, role, token string
						status            int
						code              string
					}{"device_token", "device", "", 401, "UNAUTHENTICATED"},
				)
				if csrf {
					testCases = append(testCases,
						struct {
							name, role, token string
							status            int
							code              string
						}{"missing_csrf", requiredRole, "", 403, "CSRF_FAILED"},
						struct {
							name, role, token string
							status            int
							code              string
						}{"forged_csrf", requiredRole, "forged", 403, "CSRF_FAILED"},
					)
				}
			} else if deviceOnly {
				for _, role := range []string{"", "merchant", "admin"} {
					testCases = append(testCases, struct {
						name, role, token string
						status            int
						code              string
					}{"missing_device_" + role, role, "", 401, "DEVICE_UNAUTHORIZED"})
				}
			} else {
				testCases = append(testCases, struct {
					name, role, token string
					status            int
					code              string
				}{"missing_signature", "", "", 200, "INVALID_SIGNATURE"})
			}
			for _, test := range testCases {
				cases++
				t.Run(method+" "+path+"/"+test.name, func(t *testing.T) {
					headers := http.Header{"Content-Type": {"application/json"}}
					if proof, ok := proofs[test.role]; ok {
						headers.Set("Cookie", proof.cookie.Name+"="+proof.cookie.Value)
					}
					if test.role == "device" {
						headers.Set("Authorization", "Bearer "+fixture.DeviceToken)
					}
					if test.token != "" {
						headers.Set("X-CSRF-Token", test.token)
					}
					requestBody := body
					if protocol {
						headers.Set("Content-Type", "application/x-www-form-urlencoded")
						requestBody = nil
					}
					response := fixture.RequestWithHeaders(t, method, target, bytes.NewReader(requestBody), "guest", headers)
					if response.Code != test.status {
						t.Fatalf("authorization status=%d want=%d", response.Code, test.status)
					}
					var envelope struct {
						Error *struct {
							Code string `json:"code"`
						} `json:"error"`
						ErrorCode string `json:"error_code"`
					}
					if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
						t.Fatal(err)
					}
					code := envelope.ErrorCode
					if envelope.Error != nil {
						code = envelope.Error.Code
					}
					if code != test.code {
						t.Fatalf("authorization code=%q want=%q", code, test.code)
					}
				})
			}
		}
	}
	t.Logf("permission matrix: %d cases", cases)
}

func TestHTTP_OpenAPISessionCookiesAndLogout(t *testing.T) {
	fixture, proofs := securityFixture(t)
	for role, proof := range proofs {
		t.Run(role, func(t *testing.T) {
			cookie := proof.cookie
			if !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" || !strings.HasPrefix(cookie.Name, "__Host-") {
				t.Fatal("production session cookie lacks its required security attributes")
			}
			prefix, logout := "merchant", "auth"
			if role == "admin" {
				prefix, logout = "console", "console"
			}
			headers := http.Header{"Cookie": {cookie.Name + "=" + cookie.Value}}
			headers.Set("X-CSRF-Token", proof.csrf)
			if got := fixture.RequestWithHeaders(t, http.MethodGet, "/api/v1/"+prefix+"/me", nil, "guest", headers).Code; got != 200 {
				t.Fatalf("session positive control=%d", got)
			}
			response := fixture.RequestWithHeaders(t, http.MethodPost, "/api/v1/"+logout+"/logout", strings.NewReader(`{}`), "guest", headers)
			if response.Code != 200 {
				t.Fatalf("logout status=%d", response.Code)
			}
			expired := false
			for _, cleared := range response.Result().Cookies() {
				if cleared.Name == cookie.Name && cleared.MaxAge < 0 && cleared.Value == "" {
					expired = true
				}
			}
			if !expired {
				t.Fatal("logout did not clear its session cookie")
			}
			if got := fixture.RequestWithHeaders(t, http.MethodGet, "/api/v1/"+prefix+"/me", nil, "guest", headers).Code; got != 401 {
				t.Fatalf("replayed session after logout=%d want=401", got)
			}
		})
	}
}

func securityFixture(t *testing.T) (*testsupport.HTTPFixture, map[string]sessionProof) {
	t.Helper()
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	password := "Aa1!" + rand.Text()
	code, err := fixture.App.Services.Auth.RotateBootstrapCode(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	admin := fixture.Request(t, http.MethodPost, "/api/v1/console/bootstrap", auth.BootstrapRequest{InitializationCode: code, Username: "contract_admin", Password: password, PasswordConfirmation: password}, "admin:matrix-admin")
	var version int64
	if err := db.Pool.QueryRow(t.Context(), `SELECT version FROM site_settings WHERE singleton=true`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	value := json.RawMessage(`{"turnstile_site_key":"contract-site","turnstile_secret":"contract-secret","allowed_hostname":"pay.example.test","restrict_email_domains":false,"email_domains":[],"ban_contact":{"zh_CN":"","en":""}}`)
	if _, err := fixture.App.Services.Settings.UpdateGroup(t.Context(), platform.Actor{Role: "admin", SubjectID: 1}, "security", settings.UpdateGroup{ExpectedVersion: version, Value: value}); err != nil {
		t.Fatal(err)
	}
	fixture.Siteverify.SetMode("accept")
	id, _ := db.SeedMerchant(t)
	hash, err := auth.HashPassword(password, nil)
	if err != nil {
		t.Fatal(err)
	}
	var email string
	if err := db.Pool.QueryRow(t.Context(), `UPDATE merchants SET password_hash=$1 WHERE id=$2 RETURNING email_normalized`, hash, id).Scan(&email); err != nil {
		t.Fatal(err)
	}
	merchant := fixture.Request(t, http.MethodPost, "/api/v1/auth/login", auth.LoginRequest{Identifier: email, Password: password, TurnstileToken: "ok:login"}, "merchant:matrix-merchant")
	proofs := map[string]sessionProof{}
	for role, response := range map[string]*httptest.ResponseRecorder{"admin": admin, "merchant": merchant} {
		var envelope struct {
			Data struct {
				CSRF string `json:"csrf_token"`
			} `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		for _, cookie := range response.Result().Cookies() {
			if cookie.HttpOnly {
				proofs[role] = sessionProof{cookie: cookie, csrf: envelope.Data.CSRF}
			}
		}
		if proofs[role].cookie == nil || proofs[role].csrf == "" {
			t.Fatal("session fixture did not authenticate")
		}
	}
	binding := fixture.Request(t, http.MethodPost, "/api/v1/merchant/device/bindings", map[string]any{"replace_confirmed": false}, "merchant:matrix-merchant")
	var bind struct {
		Data device.BindingDTO `json:"data"`
	}
	if err := json.Unmarshal(binding.Body.Bytes(), &bind); err != nil {
		t.Fatal(err)
	}
	credential := fixture.Request(t, http.MethodPost, "/api/v1/device/bind", device.RedeemRequest{Code: bind.Data.Code, DeviceName: "contract device", AppVersion: "1.0"}, "guest")
	var result struct {
		Data device.CredentialDTO `json:"data"`
	}
	if err := json.Unmarshal(credential.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	fixture.DeviceToken = result.Data.DeviceToken
	if fixture.DeviceToken == "" {
		t.Fatal("valid bound device token was not issued")
	}
	if response := fixture.Request(t, http.MethodPost, "/api/v1/device/heartbeat", device.HeartbeatRequest{AppVersion: "1.0"}, "device"); response.Code != 200 {
		t.Fatalf("valid device control=%d", response.Code)
	}
	return fixture, proofs
}
