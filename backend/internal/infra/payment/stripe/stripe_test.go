package stripe

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	appPayment "github.com/yishuiliunian/nexusapi/backend/internal/app/payment"
	dpayment "github.com/yishuiliunian/nexusapi/backend/internal/domain/payment"
)

// ---------- CreateCheckout ----------

func TestCreateCheckout_BuildsCorrectForm(t *testing.T) {
	var seenAuth, seenCT string
	var seenForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenCT = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		seenForm, _ = url.ParseQuery(string(body))
		_, _ = w.Write([]byte(`{"id":"cs_test_123","url":"https://pay.stripe.com/test_session/xyz"}`))
	}))
	defer srv.Close()

	g := New(Config{
		SecretKey:  "sk_test_xxx",
		SuccessURL: "https://app.example.com/ok",
		CancelURL:  "https://app.example.com/cancel",
		APIBase:    srv.URL,
	})

	order := &dpayment.Order{
		ID:          "local-1",
		UserID:      42,
		Amount:      20_000_000,
		AmountCents: 2000,
		Currency:    "USD",
	}
	if err := g.CreateCheckout(context.Background(), order); err != nil {
		t.Fatalf("create checkout: %v", err)
	}

	if seenAuth != "Bearer sk_test_xxx" {
		t.Errorf("auth: %q", seenAuth)
	}
	if !strings.HasPrefix(seenCT, "application/x-www-form-urlencoded") {
		t.Errorf("content-type: %q", seenCT)
	}
	if seenForm.Get("client_reference_id") != "local-1" {
		t.Errorf("client_reference_id: %q", seenForm.Get("client_reference_id"))
	}
	if seenForm.Get("line_items[0][price_data][unit_amount]") != "2000" {
		t.Errorf("unit_amount: %q", seenForm.Get("line_items[0][price_data][unit_amount]"))
	}
	if seenForm.Get("line_items[0][price_data][currency]") != "usd" {
		t.Errorf("currency: %q", seenForm.Get("line_items[0][price_data][currency]"))
	}
	if order.GatewayRef != "cs_test_123" {
		t.Errorf("gateway ref: %q", order.GatewayRef)
	}
	if order.CheckoutURL != "https://pay.stripe.com/test_session/xyz" {
		t.Errorf("url: %q", order.CheckoutURL)
	}
}

func TestCreateCheckout_UpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid_amount"}}`))
	}))
	defer srv.Close()

	g := New(Config{SecretKey: "sk", APIBase: srv.URL})
	err := g.CreateCheckout(context.Background(), &dpayment.Order{
		ID: "o", AmountCents: 100, Currency: "USD",
	})
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Errorf("want 400 error, got %v", err)
	}
}

func TestCreateCheckout_RequiresSecretKey(t *testing.T) {
	g := New(Config{}) // no secret
	err := g.CreateCheckout(context.Background(), &dpayment.Order{ID: "x", AmountCents: 1, Currency: "USD"})
	if err == nil {
		t.Error("should error without secret")
	}
}

// ---------- ParseWebhook ----------

// helper: build valid signed webhook body
func signedBody(t *testing.T, secret string, payloadJSON string, ts int64) (body []byte, header string) {
	t.Helper()
	body = []byte(payloadJSON)
	signedPayload := fmt.Sprintf("%d.%s", ts, body)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(signedPayload))
	v1 := hex.EncodeToString(mac.Sum(nil))
	header = fmt.Sprintf("t=%d,v1=%s", ts, v1)
	return
}

func TestParseWebhook_ValidSignature(t *testing.T) {
	secret := "whsec_test"
	payload := `{"id":"evt_1","type":"checkout.session.completed","data":{"object":{"id":"cs_123","client_reference_id":"local-42","payment_intent":"pi_1"}}}`
	body, sig := signedBody(t, secret, payload, time.Now().Unix())

	g := New(Config{WebhookSecret: secret})
	evt, err := g.ParseWebhook(context.Background(), body, sig)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Type != appPayment.EventCheckoutCompleted {
		t.Errorf("type=%s", evt.Type)
	}
	if evt.OrderID != "local-42" {
		t.Errorf("order id=%q", evt.OrderID)
	}
	if evt.RefID != "cs_123" {
		t.Errorf("ref id=%q", evt.RefID)
	}
}

func TestParseWebhook_TamperedBodyFails(t *testing.T) {
	secret := "whsec_test"
	payload := `{"id":"evt_1","type":"checkout.session.completed","data":{"object":{}}}`
	body, sig := signedBody(t, secret, payload, time.Now().Unix())
	body[10] ^= 0x01 // 篡改

	g := New(Config{WebhookSecret: secret})
	_, err := g.ParseWebhook(context.Background(), body, sig)
	if err == nil {
		t.Error("tampered body should fail")
	}
}

func TestParseWebhook_WrongSecretFails(t *testing.T) {
	body, sig := signedBody(t, "right_secret", `{"type":"x"}`, time.Now().Unix())
	g := New(Config{WebhookSecret: "wrong_secret"})
	if _, err := g.ParseWebhook(context.Background(), body, sig); err == nil {
		t.Error("wrong secret should fail")
	}
}

func TestParseWebhook_OldTimestampFails(t *testing.T) {
	secret := "s"
	body, sig := signedBody(t, secret, `{"type":"checkout.session.completed","data":{"object":{}}}`, time.Now().Add(-30*time.Minute).Unix())
	g := New(Config{WebhookSecret: secret, ClockSkewAllow: 5 * time.Minute})
	_, err := g.ParseWebhook(context.Background(), body, sig)
	if err == nil {
		t.Error("stale ts should fail")
	}
}

func TestParseWebhook_MalformedHeaderFails(t *testing.T) {
	g := New(Config{WebhookSecret: "s"})
	if _, err := g.ParseWebhook(context.Background(), []byte(`{}`), "garbage"); err == nil {
		t.Error("malformed header should fail")
	}
}

func TestParseWebhook_UnknownTypeNoOrderID(t *testing.T) {
	secret := "s"
	// Type 非关心：应返回 EventUnknown，OrderID 空
	body, sig := signedBody(t, secret, `{"id":"evt_x","type":"customer.created","data":{"object":{}}}`, time.Now().Unix())
	g := New(Config{WebhookSecret: secret})
	evt, err := g.ParseWebhook(context.Background(), body, sig)
	if err != nil {
		t.Fatalf("should not error on unknown type, got %v", err)
	}
	if evt.Type != appPayment.EventUnknown {
		t.Errorf("unknown type should map to EventUnknown, got %s", evt.Type)
	}
}

func TestParseWebhook_ExpiredMaps(t *testing.T) {
	secret := "s"
	body, sig := signedBody(t, secret, `{"type":"checkout.session.expired","data":{"object":{"client_reference_id":"abc"}}}`, time.Now().Unix())
	g := New(Config{WebhookSecret: secret})
	evt, _ := g.ParseWebhook(context.Background(), body, sig)
	if evt.Type != appPayment.EventCheckoutExpired {
		t.Errorf("type=%s", evt.Type)
	}
}

// 保证解析后的 JSON 结构被正确反序列化（defensive test 防手写 struct 名错漏）。
func TestParseWebhook_DecodesClientRefFromDataObject(t *testing.T) {
	secret := "s"
	obj := map[string]any{
		"id":                  "cs_abc",
		"client_reference_id": "local-xyz",
	}
	body, _ := json.Marshal(map[string]any{
		"id":   "evt_1",
		"type": "checkout.session.completed",
		"data": map[string]any{"object": obj},
	})
	signed, sig := signedBody(t, secret, string(body), time.Now().Unix())
	g := New(Config{WebhookSecret: secret})
	evt, err := g.ParseWebhook(context.Background(), signed, sig)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.OrderID != "local-xyz" {
		t.Errorf("OrderID=%q", evt.OrderID)
	}
}
