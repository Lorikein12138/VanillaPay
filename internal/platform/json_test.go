package platform

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeStrictJSON(t *testing.T) {
	type payload struct {
		Confirm *bool `json:"confirm"`
		Name    struct {
			En    string  `json:"en"`
			Alias *string `json:"alias"`
		} `json:"name,omitempty"`
		Entries []struct {
			ZhCN string `json:"zh_CN"`
		} `json:"entries,omitempty"`
		Value json.RawMessage `json:"value,omitempty"`
	}
	for _, test := range []struct {
		name, body, field string
		valid             bool
	}{
		{"exact tags", `{"confirm":false,"entries":[{"zh_CN":"内容"}],"value":{"freeKey":1}}`, "", true},
		{"present null", `{"confirm":null}`, "", true},
		{"nested pointer null", `{"confirm":false,"name":{"en":"","alias":null}}`, "", true},
		{"nested object missing key", `{"confirm":false,"name":{"alias":null}}`, "name.en", false},
		{"nested array missing key", `{"confirm":false,"entries":[{"zh_CN":"内容"},{}]}`, "entries[1].zh_CN", false},
		{"nested nonpointer null", `{"confirm":false,"name":{"en":null,"alias":null}}`, "name.en", false},
		{"array element null", `{"confirm":false,"entries":[null]}`, "entries[0]", false},
		{"missing", `{}`, "confirm", false},
		{"case variant", `{"Confirm":true}`, "Confirm", false},
		{"unknown", `{"confirm":true,"extra":null}`, "extra", false},
		{"duplicate", `{"confirm":false,"confirm":true}`, "confirm", false},
		{"escaped duplicate", `{"confirm":false,"conf\u0069rm":true}`, "confirm", false},
		{"nested case variant", `{"confirm":true,"entries":[{"zh_cn":"内容"}]}`, "entries[0].zh_cn", false},
		{"duplicate in raw map", `{"confirm":true,"value":{"items":[{"key":1,"key":2}]}}`, "value.items[0].key", false},
		{"wrong type", `{"confirm":"true"}`, "confirm", false},
		{"trailing body", `{"confirm":true}{}`, "", false},
		{"not an object", `null`, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var dst payload
			err := DecodeStrictJSON(strings.NewReader(test.body), &dst)
			if test.valid {
				if err != nil {
					t.Fatal(err)
				}
				if test.name == "exact tags" && (dst.Confirm == nil || *dst.Confirm || len(dst.Entries) != 1 ||
					dst.Entries[0].ZhCN != "内容" || string(dst.Value) != `{"freeKey":1}`) {
					t.Fatal("decoded payload does not match the request")
				}
				if test.name == "present null" && dst.Confirm != nil {
					t.Fatal("explicit null changed to a boolean")
				}
				return
			}
			var problem *Error
			if !errors.As(err, &problem) || problem.HTTPStatus != http.StatusBadRequest || problem.Code != "VALIDATION_ERROR" ||
				(test.field != "" && problem.Fields[test.field] == "") {
				t.Fatalf("expected validation error for %q, got %#v", test.field, err)
			}
			if dst.Confirm != nil {
				t.Fatal("invalid request was decoded")
			}
		})
	}
}

func TestDecodeJSONBodyEnforcesLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{  } "))
	if err := DecodeJSONBody(httptest.NewRecorder(), req, 4, &struct{}{}); err == nil {
		t.Fatal("accepted a request larger than the endpoint limit")
	}
}
