package contracts

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"vanillapay/internal/device"
	"vanillapay/internal/platform"
	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_DeviceBindingCredentialRejections(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	for _, code := range []string{"malformed", "ABCDEFGHJKLM"} {
		t.Run(code, func(t *testing.T) {
			response := fixture.Request(t, http.MethodPost, "/api/v1/device/bind", device.RedeemRequest{Code: code, DeviceName: "contract device", AppVersion: "1.0"}, "guest")
			assertAPIError(t, response, http.StatusUnauthorized, "INVALID_CODE")
		})
	}
	id, _ := db.SeedMerchant(t)
	binding, err := fixture.App.Services.Device.NewBinding(t.Context(), platform.Actor{Role: "merchant", SubjectID: id, MerchantID: id}, false)
	if err != nil {
		t.Fatal(err)
	}
	response := fixture.Request(t, http.MethodPost, "/api/v1/device/bind", device.RedeemRequest{Code: binding.Code, DeviceName: "contract device", AppVersion: "1.0"}, "guest")
	var envelope struct {
		Data device.CredentialDTO `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || response.Code != http.StatusCreated || envelope.Data.DeviceToken == "" {
		t.Fatal("valid binding must issue a device credential with status 201")
	}
}

func TestHTTP_DeviceSelfUnbindRevokesToken(t *testing.T) {
	db := testsupport.NewDatabase(t)
	fixture := testsupport.NewHTTPFixture(t, db)
	id, _ := db.SeedMerchant(t)
	binding, err := fixture.App.Services.Device.NewBinding(t.Context(), platform.Actor{Role: "merchant", SubjectID: id, MerchantID: id}, false)
	if err != nil {
		t.Fatal(err)
	}
	credential := fixture.Request(t, http.MethodPost, "/api/v1/device/bind", device.RedeemRequest{
		Code: binding.Code, DeviceName: "contract device", AppVersion: "1.0.0",
	}, "guest")
	var bound struct {
		Data device.CredentialDTO `json:"data"`
	}
	if err := json.Unmarshal(credential.Body.Bytes(), &bound); err != nil || bound.Data.DeviceToken == "" {
		t.Fatal("binding did not issue a device credential")
	}
	role := "device:" + bound.Data.DeviceToken
	unbind := fixture.Request(t, http.MethodPost, "/api/v1/device/unbind", nil, role)
	if unbind.Code != http.StatusNoContent || unbind.Body.Len() != 0 {
		t.Fatalf("unbind status=%d, size=%d", unbind.Code, unbind.Body.Len())
	}
	for _, path := range []string{"/api/v1/device/heartbeat", "/api/v1/device/receipts", "/api/v1/device/unbind"} {
		assertAPIError(t, fixture.Request(t, http.MethodPost, path, nil, role), http.StatusUnauthorized, "DEVICE_UNAUTHORIZED")
	}
	var audits, mail int
	err = db.Pool.QueryRow(t.Context(), `SELECT
		(SELECT count(*) FROM audit_logs WHERE action='device.revoke' AND actor_role='device' AND merchant_id=$1 AND target_id=$2),
		(SELECT count(*) FROM mail_deliveries WHERE template='device_offline')`, id, bound.Data.DeviceID).Scan(&audits, &mail)
	if err != nil || audits != 1 || mail != 0 {
		t.Fatalf("unbind audits=%d offline mail=%d err=%v", audits, mail, err)
	}
}

func TestHTTP_ImageUploadValidationResponses(t *testing.T) {
	fixture, _ := securityFixture(t)
	for _, endpoint := range []struct{ method, path, role string }{
		{http.MethodPost, "/api/v1/console/assets/logo", "admin:matrix-admin"},
		{http.MethodPut, "/api/v1/merchant/collection/codes/alipay", "merchant:matrix-merchant"},
		{http.MethodPut, "/api/v1/merchant/collection/codes/wxpay", "merchant:matrix-merchant"},
		{http.MethodPut, "/api/v1/merchant/collection/codes/qqpay", "merchant:matrix-merchant"},
	} {
		for _, file := range []struct {
			name, code string
			contents   []byte
			status     int
		}{
			{"not_image", "UNSUPPORTED_MEDIA_TYPE", []byte("not an image"), http.StatusUnsupportedMediaType},
			{"too_large", "UPLOAD_TOO_LARGE", make([]byte, 5*1024*1024+1), http.StatusRequestEntityTooLarge},
		} {
			t.Run(endpoint.path+"/"+file.name, func(t *testing.T) {
				var body bytes.Buffer
				writer := multipart.NewWriter(&body)
				part, err := writer.CreateFormFile("file", "upload.png")
				if err != nil {
					t.Fatal(err)
				}
				if _, err := part.Write(file.contents); err != nil {
					t.Fatal(err)
				}
				if err := writer.Close(); err != nil {
					t.Fatal(err)
				}
				if body.Len() >= (5<<20)+(64<<10) {
					t.Fatal("multipart envelope must reach the file-size validation in the service")
				}
				headers := http.Header{"Content-Type": {writer.FormDataContentType()}}
				response := fixture.RequestWithHeaders(t, endpoint.method, endpoint.path, &body, endpoint.role, headers)
				assertAPIError(t, response, file.status, file.code)
			})
		}
	}
}

func TestHTTP_AnnouncementReadRejectsStaleRevision(t *testing.T) {
	fixture, _ := securityFixture(t)
	created := fixture.Request(t, http.MethodPost, "/api/v1/console/announcements", map[string]any{
		"title": map[string]string{"zh_CN": "契约回归", "en": "Contract regression"},
		"body":  map[string]string{"zh_CN": "公告正文", "en": "Announcement body"},
		"popup": false,
	}, "admin:matrix-admin")
	var envelope struct {
		Data struct {
			ID       string `json:"id"`
			Revision int64  `json:"revision"`
		} `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &envelope); err != nil || created.Code != http.StatusCreated || envelope.Data.ID == "" {
		t.Fatal("create announcement positive control")
	}
	path := "/api/v1/merchant/announcements/" + envelope.Data.ID + "/read"
	stale := fixture.Request(t, http.MethodPost, path, map[string]any{"revision": envelope.Data.Revision + 1}, "merchant:matrix-merchant")
	assertAPIError(t, stale, http.StatusConflict, "VERSION_CONFLICT")
	current := fixture.Request(t, http.MethodPost, path, map[string]any{"revision": envelope.Data.Revision}, "merchant:matrix-merchant")
	if current.Code != http.StatusOK {
		t.Fatalf("current announcement revision status=%d", current.Code)
	}
}

func assertAPIError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	var envelope struct {
		Error struct{ Code string } `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if response.Code != status || envelope.Error.Code != code {
		t.Fatalf("response status=%d code=%q, want %d/%s", response.Code, envelope.Error.Code, status, code)
	}
}
