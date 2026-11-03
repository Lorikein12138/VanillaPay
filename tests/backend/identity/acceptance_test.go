package identity

import (
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"testing"

	"vanillapay/internal/auth"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_IdentityRemainingAcceptance(t *testing.T) {
	var concurrentBootstrapLosers int
	t.Run("V06.B.r01/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, _ := db.SeedMerchant(t)
		fixture := testsupport.NewHTTPFixture(t, db)
		code, err := fixture.App.Services.Auth.RotateBootstrapCode(t.Context())
		if err != nil {
			t.Fatal("prepare one-use bootstrap code")
		}

		barrier := testsupport.NewBarrier(2)
		type bootstrapResult struct {
			client  string
			status  int
			body    []byte
			cookies []*http.Cookie
			err     error
		}
		responses := make(chan bootstrapResult, 2)
		var workers sync.WaitGroup
		for _, client := range []string{"bootstrap-a", "bootstrap-b"} {

			workers.Add(1)
			go func() {
				defer workers.Done()
				if err := barrier.Wait(t.Context()); err != nil {
					responses <- bootstrapResult{client: client, err: err}
					return
				}
				request := map[string]any{
					"initialization_code":   code,
					"username":              "identity_bootstrap_" + client,
					"password":              fixturePassword,
					"password_confirmation": fixturePassword,
				}
				recorder := fixture.Request(t, http.MethodPost, "/api/v1/console/bootstrap", request, "admin:"+client)
				responses <- bootstrapResult{client: client, status: recorder.Code, body: append([]byte(nil), recorder.Body.Bytes()...), cookies: recorder.Result().Cookies()}
			}()
		}
		workers.Wait()
		close(responses)

		var winners int
		alreadyInitialized := &concurrentBootstrapLosers
		var winner string
		for result := range responses {
			if result.err != nil {
				t.Fatalf("concurrent bootstrap barrier: %v", result.err)
			}
			var envelope struct {
				Data struct {
					User  json.RawMessage `json:"user"`
					Token json.RawMessage `json:"token"`
				} `json:"data"`
				Error *struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(result.body, &envelope); err != nil {
				t.Fatalf("decode concurrent bootstrap response: %v", err)
			}
			if envelope.Error == nil {
				var user struct {
					ID   string `json:"id"`
					Role string `json:"role"`
				}
				if err := json.Unmarshal(envelope.Data.User, &user); err != nil {
					t.Fatalf("decode bootstrap user DTO: %v", err)
				}
				cookieOK := false
				for _, cookie := range result.cookies {
					if cookie.Name == "__Host-vp_console" && cookie.Value != "" && cookie.Path == "/" && cookie.HttpOnly && cookie.Secure && cookie.SameSite == http.SameSiteLaxMode {
						cookieOK = true
					}
				}
				if user.ID == "1" && user.Role == "admin" && len(envelope.Data.Token) == 0 && cookieOK {
					if result.status != http.StatusCreated {
						t.Errorf("successful bootstrap status=%d, want %d", result.status, http.StatusCreated)
					}
					winners++
					winner = result.client
					continue
				}
				t.Fatalf("concurrent bootstrap success violated the session contract for %s: %s", result.client, result.body)
			}
			if envelope.Error != nil && envelope.Error.Code == "ALREADY_INITIALIZED" {
				if result.status != http.StatusConflict {
					t.Errorf("losing bootstrap status=%d, want %d", result.status, http.StatusConflict)
				}
				(*alreadyInitialized)++
				continue
			}
			t.Fatalf("concurrent bootstrap outcome for %s = %s", result.client, result.body)
		}
		if winners != 1 || *alreadyInitialized != 1 {
			t.Fatalf("concurrent bootstrap winners=%d already_initialized=%d; want one each", winners, *alreadyInitialized)
		}
		var adminCount int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM admins WHERE id=1`).Scan(&adminCount); err != nil || adminCount != 1 {
			t.Fatalf("bootstrap created admin id=1 count=%d err=%v", adminCount, err)
		}
		var merchantCount int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM merchants WHERE id=$1`, merchantID).Scan(&merchantCount); err != nil || merchantCount != 1 {
			t.Fatalf("bootstrap occupied or removed merchant ID %d: count=%d err=%v", merchantID, merchantCount, err)
		}
		me := fixture.Request(t, http.MethodGet, "/api/v1/console/me", nil, "admin:"+winner)
		if me.Code != http.StatusOK {
			t.Fatalf("successful bootstrap cookie did not authenticate console/me: status=%d body=%s", me.Code, me.Body.String())
		}
	})
	t.Run("V06.B.r01/v02", func(t *testing.T) {
		if concurrentBootstrapLosers != 1 {
			t.Fatalf("concurrent bootstrap ALREADY_INITIALIZED losers=%d, want exactly one", concurrentBootstrapLosers)
		}
	})

	t.Run("V06.B.r02/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		fixture := testsupport.NewHTTPFixture(t, db)
		code, err := fixture.App.Services.Auth.RotateBootstrapCode(t.Context())
		if err != nil {
			t.Fatal("prepare invalid-code bootstrap fixture")
		}
		for _, submitted := range []string{"wrong-initialization-code"} {
			response := fixture.Request(t, http.MethodPost, "/api/v1/console/bootstrap", map[string]any{
				"initialization_code":   submitted,
				"username":              "invalid_bootstrap_admin",
				"password":              fixturePassword,
				"password_confirmation": fixturePassword,
			}, "guest")
			if response.Code != http.StatusBadRequest {
				t.Fatalf("invalid bootstrap code status=%d body=%s", response.Code, response.Body.String())
			}
			var envelope struct {
				Error *struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || envelope.Error == nil || envelope.Error.Code != "INVALID_CODE" {
				t.Fatalf("invalid or previously consumed code response=%s err=%v", response.Body.String(), err)
			}
		}
		created := fixture.Request(t, http.MethodPost, "/api/v1/console/bootstrap", map[string]any{
			"initialization_code":   code,
			"username":              "valid_bootstrap_admin",
			"password":              fixturePassword,
			"password_confirmation": fixturePassword,
		}, "admin:valid-bootstrap")
		if created.Code != http.StatusCreated {
			t.Fatalf("valid bootstrap status=%d body=%s; design requires one HTTP 201", created.Code, created.Body.String())
		}
		replay := fixture.Request(t, http.MethodPost, "/api/v1/console/bootstrap", map[string]any{
			"initialization_code":   code,
			"username":              "replayed_bootstrap_admin",
			"password":              fixturePassword,
			"password_confirmation": fixturePassword,
		}, "guest")
		var replayEnvelope struct {
			Error *struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal(replay.Body.Bytes(), &replayEnvelope); err != nil || replayEnvelope.Error == nil || replayEnvelope.Error.Code != "ALREADY_INITIALIZED" || replay.Code != http.StatusConflict {
			t.Fatalf("consumed bootstrap code replay status=%d body=%s err=%v", replay.Code, replay.Body.String(), err)
		}
		var admins int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM admins`).Scan(&admins); err != nil || admins != 1 {
			t.Fatalf("invalid/replayed bootstrap code changed the single initialized admin count=%d err=%v", admins, err)
		}
	})
}

func TestDB_BootstrapRotationAndIdentitySequences(t *testing.T) {
	t.Run("V06.B.r03/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		application := db.NewApplication(t)
		oldCode, err := application.Services.Auth.RotateBootstrapCode(t.Context())
		if err != nil || oldCode == "" {
			t.Fatalf("issue original bootstrap code: code=%q err=%v", oldCode, err)
		}
		newCode, err := application.Services.Auth.RotateBootstrapCode(t.Context())
		if err != nil || newCode == "" || newCode == oldCode {
			t.Fatalf("rotate bootstrap code: old=%q new=%q err=%v", oldCode, newCode, err)
		}
		request := func(code, username string) auth.BootstrapRequest {
			return auth.BootstrapRequest{InitializationCode: code, Username: username, Password: fixturePassword, PasswordConfirmation: fixturePassword}
		}
		if _, err := application.Services.Auth.Bootstrap(t.Context(), request(oldCode, "stale_bootstrap_admin")); !hasIdentityError(err, "INVALID_CODE") {
			t.Fatalf("rotated-away bootstrap code error=%v, want INVALID_CODE", err)
		}
		if _, err := application.Services.Auth.Bootstrap(t.Context(), request(newCode, "current_bootstrap_admin")); err != nil {
			t.Fatalf("current bootstrap code was not accepted: %v", err)
		}
	})

	t.Run("V06.B.r03/v02", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		application := db.NewApplication(t)
		code, err := application.Services.Auth.RotateBootstrapCode(t.Context())
		if err != nil || code == "" {
			t.Fatalf("issue bootstrap code: code=%q err=%v", code, err)
		}
		request := auth.BootstrapRequest{InitializationCode: code, Username: "initialized_admin", Password: fixturePassword, PasswordConfirmation: fixturePassword}
		if _, err := application.Services.Auth.Bootstrap(t.Context(), request); err != nil {
			t.Fatalf("initialize administrator: %v", err)
		}
		rotated, err := application.Services.Auth.RotateBootstrapCode(t.Context())
		if err != nil || rotated != "" {
			t.Fatalf("initialized service exposed a fresh bootstrap code: code=%q err=%v", rotated, err)
		}
		if _, err := application.Services.Auth.Bootstrap(t.Context(), request); !hasIdentityError(err, "ALREADY_INITIALIZED") {
			t.Fatalf("initialized service accepted bootstrap again: %v", err)
		}
	})

	t.Run("V06.B.r04/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		application := db.NewApplication(t)
		code, err := application.Services.Auth.RotateBootstrapCode(t.Context())
		if err != nil {
			t.Fatal("issue bootstrap code")
		}
		if _, err := application.Services.Auth.Bootstrap(t.Context(), auth.BootstrapRequest{InitializationCode: code, Username: "only_admin", Password: fixturePassword, PasswordConfirmation: fixturePassword}); err != nil {
			t.Fatalf("bootstrap admin: %v", err)
		}
		var total, idOne int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*),count(*) FILTER (WHERE id=1) FROM admins`).Scan(&total, &idOne); err != nil || total != 1 || idOne != 1 {
			t.Fatalf("admin identity sequence total=%d id_one=%d err=%v; want exactly admin id=1", total, idOne, err)
		}
	})

	t.Run("V06.B.r04/v02", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		merchantID, _ := db.SeedMerchant(t)
		application := db.NewApplication(t)
		code, err := application.Services.Auth.RotateBootstrapCode(t.Context())
		if err != nil {
			t.Fatal("issue bootstrap code")
		}
		if _, err := application.Services.Auth.Bootstrap(t.Context(), auth.BootstrapRequest{InitializationCode: code, Username: "independent_admin", Password: fixturePassword, PasswordConfirmation: fixturePassword}); err != nil {
			t.Fatalf("bootstrap admin: %v", err)
		}
		var count int
		if err := db.Pool.QueryRow(t.Context(), `SELECT count(*) FROM merchants WHERE id=$1`, merchantID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("admin bootstrap occupied merchant ID %d: count=%d err=%v", merchantID, count, err)
		}
	})
}

func hasIdentityError(err error, want string) bool {
	var target *platform.Error
	return errors.As(err, &target) && target.Code == want
}
