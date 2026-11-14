package contracts

import (
	"encoding/json"
	"mime"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"vanillapay/internal/auth"
	"vanillapay/internal/device"
	"vanillapay/internal/merchant"
	"vanillapay/internal/payment"
	"vanillapay/internal/platform"
	"vanillapay/internal/portal"
	"vanillapay/internal/settings"
	"vanillapay/internal/subscription"
	"vanillapay/tests/backend/testsupport"
)

func TestOpenAPI_JSONRequestTypes(t *testing.T) {
	// Keys are HTTP method and path relative to /api/v1. Values are the types
	// the handlers pass to the strict decoder.
	requests := map[string]any{
		"POST /console/bootstrap":                          auth.BootstrapRequest{},
		"POST /auth/login":                                 auth.LoginRequest{},
		"POST /auth/second-factor":                         auth.FactorRequest{},
		"POST /console/login":                              auth.LoginRequest{},
		"POST /console/second-factor":                      auth.FactorRequest{},
		"POST /console/recovery":                           auth.RecoveryRequest{},
		"POST /auth/register":                              auth.RegisterRequest{},
		"POST /auth/reset-password":                        auth.ResetPasswordRequest{},
		"POST /auth/email-codes":                           auth.EmailCodeRequest{},
		"PATCH /merchant/preferences":                      auth.PreferencesRequest{},
		"POST /merchant/2fa/setup":                         auth.TOTPSetupRequest{},
		"POST /merchant/2fa/confirm":                       auth.TOTPConfirm{},
		"DELETE /merchant/2fa":                             auth.FactorProof{},
		"PATCH /console/preferences":                       auth.PreferencesRequest{},
		"POST /console/2fa/setup":                          auth.TOTPSetupRequest{},
		"POST /console/2fa/confirm":                        auth.TOTPConfirm{},
		"DELETE /console/2fa":                              auth.FactorProof{},
		"PATCH /console/password":                          auth.AdminPasswordRequest{},
		"PATCH /merchant/profile/username":                 auth.ChangeUsernameRequest{},
		"PATCH /merchant/profile/email":                    auth.ChangeEmailRequest{},
		"PATCH /merchant/profile/password":                 auth.ChangePasswordRequest{},
		"POST /console/merchants":                          merchant.CreateMerchant{},
		"POST /console/merchants/{id}/ban":                 merchant.BanRequest{},
		"POST /console/merchants/{id}/unban":               struct{}{},
		"POST /console/merchants/{id}/approve":             struct{}{},
		"POST /console/merchants/{id}/reject":              merchant.RejectRegistrationRequest{},
		"POST /console/merchants/{id}/reset-password":      merchant.ResetPasswordRequest{},
		"POST /merchant/integration/reset-key":             merchant.ResetKeyRequest{},
		"PUT /merchant/integration/rsa-public-key":         merchant.RSAKeyRequest{},
		"PUT /merchant/integration/ip-whitelist":           merchant.IPWhitelistRequest{},
		"PUT /merchant/integration/notify-domains":         merchant.NotifyDomains{},
		"PUT /merchant/integration/callback-settings":      merchant.CallbackSettings{},
		"POST /merchant/test-payment":                      payment.TestPaymentRequest{},
		"POST /device/bind":                                device.RedeemRequest{},
		"POST /device/heartbeat":                           device.HeartbeatRequest{},
		"POST /device/receipts":                            device.ReceiptRequest{},
		"DELETE /merchant/device":                          device.UnbindRequest{},
		"POST /merchant/device/bindings":                   device.CreateBindingRequest{},
		"PUT /console/settings/{group}":                    settings.UpdateGroup{},
		"POST /console/settings/mail/test":                 settings.MailTestRequest{},
		"PATCH /merchant/collection/codes/{channel}":       settings.SetQREnabledRequest{},
		"PUT /merchant/collection/settings":                settings.SaveCollectionRequest{},
		"POST /merchant/subscription/purchases":            subscription.PurchaseRequest{},
		"POST /console/plans":                              subscription.SavePlanRequest{},
		"PUT /console/plans/{id}":                          subscription.SavePlanRequest{},
		"DELETE /console/plans/{id}":                       subscription.DeletePlanRequest{},
		"PUT /console/merchants/{id}/subscription":         subscription.GrantSubscriptionRequest{},
		"POST /merchant/notifications/read":                portal.MarkNotificationsReadRequest{},
		"POST /merchant/announcements/{id}/read":           portal.AnnouncementRevisionRequest{},
		"POST /console/announcements":                      portal.AnnouncementCommand{},
		"PUT /console/announcements/{id}":                  portal.AnnouncementCommand{},
		"DELETE /console/announcements/{id}":               portal.AnnouncementRevisionRequest{},
		"POST /merchant/orders/{trade_no}/supplement":      payment.SupplementRequest{},
		"POST /merchant/orders/{trade_no}/refund":          payment.MarkRefundedRequest{},
		"POST /merchant/refunds/{id}/resolve":              payment.RefundResolution{},
		"POST /merchant/orders/{trade_no}/callback-replay": struct{}{},
		"POST /checkout/{tradeNo}/channel":                 payment.CheckoutChannelRequest{},
		"POST /checkout/{tradeNo}/verify":                  payment.CheckoutVerifyRequest{},
	}
	seen := map[string]bool{}
	for path, item := range testsupport.OpenAPI(t).Paths.Map() {
		for method, operation := range item.Operations() {
			if operation.RequestBody == nil {
				continue
			}
			for mediaType, media := range operation.RequestBody.Value.Content {
				kind, _, _ := mime.ParseMediaType(mediaType)
				if !strings.HasSuffix(kind, "/json") && !strings.HasSuffix(kind, "+json") {
					continue
				}
				key := method + " " + strings.TrimPrefix(path, "/api/v1")
				t.Run(method+" "+path+" "+mediaType, func(t *testing.T) {
					request, ok := requests[key]
					if !ok {
						t.Fatal("JSON request has no Go type mapping")
					}
					seen[key] = true
					var variants map[string]any
					if key == "PUT /console/settings/{group}" {
						// The HTTP boundary decodes value by path group and recovery role.
						variants = map[string]any{
							"SiteGroup":             settings.SiteGroupRequest{},
							"Home":                  settings.HomeGroupRequest{},
							"SettingsFooterLegal":   settings.FooterLegal{}, // Recurses into FooterItem.Label and FooterItem.Text.
							"MailSettingsInput":     settings.MailGroupRequest{},
							"SecuritySettingsInput": settings.SecurityGroupRequest{},    // Includes the optional proof for enabling or changing Turnstile.
							"RecoverySecurityInput": settings.RecoverySecurityRequest{}, // The one-time recovery session provides its own proof.
							"SettingsGroupBounds":   settings.GroupBounds{},
							"PaymentRisk":           platform.PaymentRisk{},
							"BillingSettingsInput":  settings.BillingGroupRequest{},
						}
					}
					checkRequestSchema(t, "body", media.Schema, reflect.TypeOf(request), variants)
					if len(variants) != 0 {
						t.Errorf("body.value: Go types mapped to absent variants: %v", variants)
					}
				})
			}
		}
	}
	for key := range requests {
		if !seen[key] {
			t.Errorf("%s: Go type mapped to absent JSON request", key)
		}
	}
}

func TestOpenAPI_PublicFooterItemType(t *testing.T) {
	public := testsupport.OpenAPI(t).Components.Schemas["PublicConfig"].Value
	item := public.Properties["footer"].Value.Items
	if item == nil || item.Ref != "#/components/schemas/SettingsFooterItem" {
		t.Fatal("public footer must share the settings footer item schema")
	}
	checkRequestSchema(t, "public.footer[]", item, reflect.TypeFor[settings.FooterItem](), nil)
}

func TestOpenAPI_PaymentProtocolForms(t *testing.T) {
	forms := map[string]string{
		"/api.php":             "V1ActionForm",
		"/api/merchant/info":   "V2MerchantInfoForm",
		"/api/merchant/orders": "V2MerchantOrdersForm",
		"/api/pay/close":       "V2QueryForm",
		"/api/pay/refundquery": "V2RefundQueryForm",
	}
	paths := testsupport.OpenAPI(t).Paths
	for path, name := range forms {
		t.Run(path, func(t *testing.T) {
			op := paths.Find(path).Post
			content := op.RequestBody.Value.Content
			form := content["application/x-www-form-urlencoded"]
			if len(content) != 1 || form == nil || form.Schema.Ref != "#/components/schemas/"+name {
				t.Fatalf("protocol request must use %s form fields", name)
			}
		})
	}
}

func checkRequestSchema(t *testing.T, path string, ref *openapi3.SchemaRef, typ reflect.Type, variants map[string]any) {
	t.Helper()
	if ref == nil || ref.Value == nil {
		t.Errorf("%s: missing OpenAPI schema", path)
		return
	}
	schema := ref.Value
	pointer := typ.Kind() == reflect.Pointer
	if schema.Nullable != pointer {
		t.Errorf("%s: OpenAPI nullable=%t, Go pointer=%t (%s)", path, schema.Nullable, pointer, typ)
	}
	if pointer {
		typ = typ.Elem()
	}
	switch {
	case len(schema.OneOf) != 0:
		if typ != reflect.TypeFor[json.RawMessage]() || path != "body.value" || variants == nil {
			t.Errorf("%s: no mapping for oneOf", path)
			return
		}
		for _, variant := range schema.OneOf {
			_, name, _ := strings.Cut(variant.Ref, "#/components/schemas/")
			request, ok := variants[name]
			if !ok {
				t.Errorf("%s: no Go type for variant %s", path, variant.Ref)
				continue
			}
			delete(variants, name)
			checkRequestSchema(t, path+"<"+name+">", variant, reflect.TypeOf(request), nil)
		}
	case schema.Type.Is("object"):
		checkRequestObject(t, path, schema, typ, variants)
	case schema.Type.Is("array"):
		if typ.Kind() != reflect.Slice && typ.Kind() != reflect.Array {
			t.Errorf("%s: OpenAPI array has Go type %s", path, typ)
			return
		}
		checkRequestSchema(t, path+"[]", schema.Items, typ.Elem(), nil)
	case schema.Type.Is("string"):
		if typ.Kind() != reflect.String && typ != reflect.TypeFor[time.Time]() {
			t.Errorf("%s: OpenAPI string has Go type %s", path, typ)
		}
	case schema.Type.Is("boolean"):
		if typ.Kind() != reflect.Bool {
			t.Errorf("%s: OpenAPI boolean has Go type %s", path, typ)
		}
	case schema.Type.Is("integer"):
		if typ.Kind() < reflect.Int || typ.Kind() > reflect.Uint64 {
			t.Errorf("%s: OpenAPI integer has Go type %s", path, typ)
		}
	case schema.Type.Is("number"):
		if typ.Kind() < reflect.Int || typ.Kind() > reflect.Float64 || typ.Kind() == reflect.Uintptr {
			t.Errorf("%s: OpenAPI number has Go type %s", path, typ)
		}
	default:
		t.Errorf("%s: unsupported OpenAPI request schema type %v", path, schema.Type)
	}
}

func checkRequestObject(t *testing.T, path string, schema *openapi3.Schema, typ reflect.Type, variants map[string]any) {
	t.Helper()
	if typ.Kind() != reflect.Struct {
		t.Errorf("%s: OpenAPI object has Go type %s", path, typ)
		return
	}
	seen := map[string]bool{}
	for index := range typ.NumField() {
		field := typ.Field(index)
		tag, options, _ := strings.Cut(field.Tag.Get("json"), ",")
		if !field.IsExported() || tag == "-" {
			continue
		}
		fieldPath := path + "." + tag
		if tag == "" {
			t.Errorf("%s.%s: missing explicit Go json tag", path, field.Name)
			continue
		}
		if seen[tag] {
			t.Errorf("%s: duplicate Go json tag", fieldPath)
		}
		seen[tag] = true
		property, ok := schema.Properties[tag]
		if !ok {
			t.Errorf("%s: Go json tag absent from OpenAPI", fieldPath)
			continue
		}
		required := slices.Contains(schema.Required, tag)
		goRequired := !slices.Contains(strings.Split(options, ","), "omitempty")
		if required != goRequired {
			t.Errorf("%s: OpenAPI required=%t, Go required=%t", fieldPath, required, goRequired)
		}
		checkRequestSchema(t, fieldPath, property, field.Type, variants)
	}
	for key := range schema.Properties {
		if !seen[key] {
			t.Errorf("%s.%s: OpenAPI property has no exact Go json tag", path, key)
		}
	}
	for _, key := range schema.Required {
		if _, ok := schema.Properties[key]; !ok {
			t.Errorf("%s.%s: required OpenAPI property has no schema", path, key)
		}
	}
}
