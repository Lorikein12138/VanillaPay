package settings

import (
	"encoding/json"
	"maps"
	"net/http"
	"strings"
	"testing"

	"vanillapay/tests/backend/testsupport"
)

func TestHTTP_LegalRevisionAndFooterConfiguration(t *testing.T) {
	t.Run("V21.B.r01/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		fixture := testsupport.NewHTTPFixture(t, db)
		admin := bootstrapRecoveryAdmin(t, fixture, "legal-save")
		response := updateSettingsGroupHTTP(t, fixture, admin, "footer_legal", map[string]any{
			"items": []any{}, "terms": map[string]string{"zh_CN": "terms zh", "en": "terms en"},
			"privacy": map[string]string{"zh_CN": "privacy zh", "en": "privacy en"},
		})
		assertRecoveryAPIError(t, response, http.StatusOK, "")
		for kind, want := range map[string]string{"terms": "terms en", "privacy": "privacy en"} {
			var revision int64
			var content []byte
			if err := db.Pool.QueryRow(t.Context(), `SELECT revision,content FROM legal_versions WHERE kind=$1 ORDER BY revision DESC LIMIT 1`, kind).Scan(&revision, &content); err != nil {
				t.Fatalf("read current %s legal revision: %v", kind, err)
			}
			var localized map[string]string
			if err := json.Unmarshal(content, &localized); err != nil || revision != 1 || localized["zh_CN"] == "" || localized["en"] != want {
				t.Fatalf("saved %s revision=%d content=%#v err=%v", kind, revision, localized, err)
			}
		}
	})

	t.Run("V21.B.r01/v02", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		fixture := testsupport.NewHTTPFixture(t, db)
		admin := bootstrapRecoveryAdmin(t, fixture, "legal-public")
		response := updateSettingsGroupHTTP(t, fixture, admin, "footer_legal", map[string]any{
			"items": []any{}, "terms": map[string]string{"zh_CN": "public terms", "en": "Public terms"},
			"privacy": map[string]string{"zh_CN": "public privacy", "en": "Public privacy"},
		})
		assertRecoveryAPIError(t, response, http.StatusOK, "")
		for kind, want := range map[string]string{"terms": "Public terms", "privacy": "Public privacy"} {
			result := fixture.Request(t, http.MethodGet, "/api/v1/public/legal/"+kind, nil, "guest")
			var envelope struct {
				Data struct {
					Kind     string            `json:"kind"`
					Revision int64             `json:"revision"`
					Content  map[string]string `json:"content"`
				} `json:"data"`
			}
			if err := json.Unmarshal(result.Body.Bytes(), &envelope); err != nil || result.Code != http.StatusOK || envelope.Data.Kind != kind || envelope.Data.Revision != 1 || envelope.Data.Content["en"] != want || envelope.Data.Content["zh_CN"] == "" {
				t.Fatalf("public %s legal response status=%d data=%#v err=%v body=%s", kind, result.Code, envelope.Data, err, result.Body.String())
			}
		}
	})

	t.Run("V21.B.r03/v01", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		fixture := testsupport.NewHTTPFixture(t, db)
		admin := bootstrapRecoveryAdmin(t, fixture, "footer-visibility")
		items := []map[string]any{
			{"id": "support", "label": map[string]string{"zh_CN": "支持", "en": "Support"}, "text": map[string]string{"zh_CN": "", "en": ""}, "url": "https://support.example.test", "enabled": true, "sort": 20},
			{"id": "hidden", "label": map[string]string{"zh_CN": "隐藏", "en": "Hidden"}, "text": map[string]string{"zh_CN": "隐藏", "en": "Hidden"}, "url": nil, "enabled": false, "sort": 1},
			{"id": "terms", "label": map[string]string{"zh_CN": "条款", "en": "Terms"}, "text": map[string]string{"zh_CN": "条款页面", "en": "legal page"}, "url": "https://pay.example.test/terms", "enabled": true, "sort": 10},
		}
		response := updateSettingsGroupHTTP(t, fixture, admin, "footer_legal", map[string]any{
			"items": items, "terms": map[string]string{"zh_CN": "条款", "en": "Terms"}, "privacy": map[string]string{"zh_CN": "隐私", "en": "Privacy"},
		})
		assertRecoveryAPIError(t, response, http.StatusOK, "")
		public := fixture.Request(t, http.MethodGet, "/api/v1/public/config", nil, "guest")
		var envelope struct {
			Data struct {
				Footer []struct {
					ID   string            `json:"id"`
					Text map[string]string `json:"text"`
					URL  string            `json:"url"`
				} `json:"footer"`
			} `json:"data"`
		}
		if err := json.Unmarshal(public.Body.Bytes(), &envelope); err != nil || public.Code != http.StatusOK || len(envelope.Data.Footer) != 2 {
			t.Fatalf("public footer status=%d footer=%#v err=%v body=%s", public.Code, envelope.Data.Footer, err, public.Body.String())
		}
		terms, support := envelope.Data.Footer[0], envelope.Data.Footer[1]
		if terms.ID != "terms" || support.ID != "support" || support.URL != "https://support.example.test" ||
			!maps.Equal(terms.Text, map[string]string{"zh_CN": "条款页面", "en": "legal page"}) ||
			!maps.Equal(support.Text, map[string]string{"zh_CN": "", "en": ""}) {
			t.Fatalf("public footer did not preserve enabled/sort/text/link fields: %#v", envelope.Data.Footer)
		}
	})

	t.Run("V21.B.r03/v02", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		fixture := testsupport.NewHTTPFixture(t, db)
		admin := bootstrapRecoveryAdmin(t, fixture, "footer-scheme")
		before := readConsoleSettingsGroups(t, fixture, admin)
		version := recoverySettingsVersion(t, fixture)
		response := fixture.Request(t, http.MethodPut, "/api/v1/console/settings/footer_legal", map[string]any{
			"expected_version": version, "value": map[string]any{
				"items": []map[string]any{{"id": "danger", "label": map[string]string{"zh_CN": "危险", "en": "Danger"}, "text": map[string]string{"zh_CN": "x", "en": ""}, "url": "javascript:alert(1)", "enabled": true, "sort": 1}},
				"terms": map[string]string{"zh_CN": "条款", "en": "Terms"}, "privacy": map[string]string{"zh_CN": "隐私", "en": "Privacy"},
			},
		}, admin)
		assertRecoveryAPIError(t, response, http.StatusBadRequest, "VALIDATION_ERROR")
		after := readConsoleSettingsGroups(t, fixture, admin)
		if string(before["footer_legal"]) != string(after["footer_legal"]) || recoverySettingsVersion(t, fixture) != version {
			t.Fatalf("dangerous footer URL partially committed: before=%s after=%s", before["footer_legal"], after["footer_legal"])
		}
	})

	t.Run("V21.B.r03/v03", func(t *testing.T) {
		db := testsupport.NewDatabase(t)
		fixture := testsupport.NewHTTPFixture(t, db)
		admin := bootstrapRecoveryAdmin(t, fixture, "footer-dom-data")
		response := updateSettingsGroupHTTP(t, fixture, admin, "footer_legal", map[string]any{
			"items": []map[string]any{{"id": "dom-data", "label": map[string]string{"zh_CN": "页脚", "en": "Footer"}, "text": map[string]string{"zh_CN": "由前端渲染", "en": "rendered by frontend"}, "url": "https://pay.example.test/footer", "enabled": true, "sort": 1}},
			"terms": map[string]string{"zh_CN": "条款正文", "en": "Terms body"}, "privacy": map[string]string{"zh_CN": "隐私正文", "en": "Privacy body"},
		})
		assertRecoveryAPIError(t, response, http.StatusOK, "")
		public := fixture.Request(t, http.MethodGet, "/api/v1/public/config", nil, "guest")
		if public.Code != http.StatusOK || !strings.Contains(public.Body.String(), "rendered by frontend") {
			t.Fatalf("public config did not return the saved enabled footer item: %d %s", public.Code, public.Body.String())
		}
		legal := fixture.Request(t, http.MethodGet, "/api/v1/public/legal/terms", nil, "guest")
		if legal.Code != http.StatusOK || !strings.Contains(legal.Body.String(), "Terms body") {
			t.Fatalf("public legal endpoint did not return the saved terms body: %d %s", legal.Code, legal.Body.String())
		}
	})
}

func TestHTTP_FooterDescriptionValidation(t *testing.T) {
	fixture := testsupport.NewHTTPFixture(t, testsupport.NewDatabase(t))
	admin := bootstrapRecoveryAdmin(t, fixture, "footer-text")
	zh := strings.Repeat("收", 1000)
	en := strings.Repeat("a", 1000)
	for _, tc := range []struct {
		name   string
		text   any
		status int
	}{
		{"both at limit", map[string]string{"zh_CN": zh, "en": en}, http.StatusOK},
		{"Chinese over limit", map[string]string{"zh_CN": zh + "收", "en": en}, http.StatusBadRequest},
		{"English over limit", map[string]string{"zh_CN": zh, "en": en + "a"}, http.StatusBadRequest},
		{"missing English", map[string]string{"zh_CN": zh}, http.StatusBadRequest},
		{"old string", zh, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item := map[string]any{
				"id": "help", "label": map[string]string{"zh_CN": "帮助", "en": "Help"},
				"text": tc.text, "url": nil, "enabled": true, "sort": 0,
			}
			legal := map[string]string{"zh_CN": "协议", "en": "Legal"}
			response := updateSettingsGroupHTTP(t, fixture, admin, "footer_legal", map[string]any{
				"items": []any{item}, "terms": legal, "privacy": legal,
			})
			code := ""
			if tc.status == http.StatusBadRequest {
				code = "VALIDATION_ERROR"
			}
			assertRecoveryAPIError(t, response, tc.status, code)
		})
	}
}
