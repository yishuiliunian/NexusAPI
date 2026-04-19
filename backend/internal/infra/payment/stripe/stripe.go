// Package stripe 实现 payment.Gateway：Stripe Checkout Session + Webhook。
//
// 不依赖 stripe-go SDK，直接打 HTTPS 调用：
//   - CreateCheckout: POST /v1/checkout/sessions（application/x-www-form-urlencoded）
//   - Webhook: 通过 Stripe-Signature 头做 HMAC SHA256 校验
//
// 仅支持 payment 一次性充值；订阅（mode=subscription）由 app/subscription 模块处理。
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
	"net/url"
	"strconv"
	"strings"
	"time"

	appPayment "github.com/yishuiliunian/nexusapi/backend/internal/app/payment"
	dpayment "github.com/yishuiliunian/nexusapi/backend/internal/domain/payment"
)

// Name 网关名。
const Name = "stripe"

// Config Stripe 集成配置。
type Config struct {
	SecretKey       string        // sk_live_... 或 sk_test_...
	WebhookSecret   string        // whsec_...
	SuccessURL      string        // 充值成功跳转
	CancelURL       string        // 取消跳转
	APIBase         string        // 默认 https://api.stripe.com
	ProductName     string        // line_item 名称，默认 "NexusAPI 充值"
	HTTPClient      *http.Client
	ClockSkewAllow  time.Duration // webhook 时间戳允许的偏差，默认 5min
}

// Gateway Stripe 网关实现。
type Gateway struct {
	cfg Config
}

// New 构造。cfg.APIBase 为空时用官方默认。
func New(cfg Config) *Gateway {
	if cfg.APIBase == "" {
		cfg.APIBase = "https://api.stripe.com"
	}
	if cfg.ProductName == "" {
		cfg.ProductName = "NexusAPI 充值"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	if cfg.ClockSkewAllow == 0 {
		cfg.ClockSkewAllow = 5 * time.Minute
	}
	return &Gateway{cfg: cfg}
}

// Name 实现。
func (g *Gateway) Name() string { return Name }

// CreateCheckout 通过 Stripe API 创建一个 Checkout Session。
//
// Mode=payment：一次性付款 line_items[price_data][unit_amount]=order.AmountCents
// Mode=subscription：订阅，使用 Plan.GatewayRef 作为 line_items[price]（Stripe price id）
//
// 传给 Stripe 的 client_reference_id 为本地 order.ID，webhook 将原样回传。
// 成功后，order.GatewayRef 填入 session.id，order.CheckoutURL 填入 url。
func (g *Gateway) CreateCheckout(ctx context.Context, order *dpayment.Order) error {
	if g.cfg.SecretKey == "" {
		return fmt.Errorf("stripe: secret key not configured")
	}
	form := url.Values{}
	form.Set("client_reference_id", order.ID)
	form.Set("success_url", g.cfg.SuccessURL)
	form.Set("cancel_url", g.cfg.CancelURL)
	form.Set("payment_method_types[0]", "card")
	form.Set("metadata[order_id]", order.ID)
	form.Set("metadata[user_id]", strconv.FormatUint(order.UserID, 10))

	if order.Mode == dpayment.ModeSubscription {
		form.Set("mode", "subscription")
		// 订阅必须用已预配置的 Stripe price id（来自 plan.GatewayRef）
		if order.GatewayRef == "" {
			return fmt.Errorf("stripe subscription: plan gateway_ref (price id) 必填")
		}
		// 这里 order.GatewayRef 传入时意为"price id"；checkout 返回后会被 session id 覆盖。
		priceID := order.GatewayRef
		order.GatewayRef = "" // 清空，由返回 session.id 填回
		form.Set("line_items[0][price]", priceID)
		form.Set("line_items[0][quantity]", "1")
		if order.PlanCode != "" {
			form.Set("metadata[plan_code]", order.PlanCode)
		}
	} else {
		form.Set("mode", "payment")
		form.Set("line_items[0][quantity]", "1")
		form.Set("line_items[0][price_data][currency]", strings.ToLower(order.Currency))
		form.Set("line_items[0][price_data][unit_amount]", strconv.FormatInt(order.AmountCents, 10))
		form.Set("line_items[0][price_data][product_data][name]", g.cfg.ProductName)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		g.cfg.APIBase+"/v1/checkout/sessions", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+g.cfg.SecretKey)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := g.cfg.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("stripe checkout: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("stripe %d: %s", resp.StatusCode, truncate(string(raw), 512))
	}
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return fmt.Errorf("stripe decode: %w", err)
	}
	order.GatewayRef = out.ID
	order.CheckoutURL = out.URL
	return nil
}

// ParseWebhook 校验 Stripe-Signature 并解析事件。
//
// Stripe 签名格式：t=<unix>,v1=<hex>,v1=<hex>...
//   signed_payload = <timestamp>.<raw_body>
//   hmac_sha256(signed_payload, webhook_secret) == v1
func (g *Gateway) ParseWebhook(_ context.Context, rawBody []byte, signature string) (appPayment.WebhookEvent, error) {
	if g.cfg.WebhookSecret == "" {
		return appPayment.WebhookEvent{}, fmt.Errorf("stripe: webhook secret 未配置")
	}
	ts, sigs, err := parseSigHeader(signature)
	if err != nil {
		return appPayment.WebhookEvent{}, err
	}
	// 时间戳窗口
	now := time.Now().Unix()
	if abs64(now-ts) > int64(g.cfg.ClockSkewAllow.Seconds()) {
		return appPayment.WebhookEvent{}, fmt.Errorf("stripe: signature timestamp too old")
	}
	signedPayload := fmt.Sprintf("%d.%s", ts, rawBody)
	mac := hmac.New(sha256.New, []byte(g.cfg.WebhookSecret))
	mac.Write([]byte(signedPayload))
	expected := hex.EncodeToString(mac.Sum(nil))
	match := false
	for _, s := range sigs {
		if hmac.Equal([]byte(s), []byte(expected)) {
			match = true
			break
		}
	}
	if !match {
		return appPayment.WebhookEvent{}, fmt.Errorf("stripe: signature mismatch")
	}
	// 解析 JSON
	var wrapper struct {
		ID   string          `json:"id"`
		Type string          `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rawBody, &wrapper); err != nil {
		return appPayment.WebhookEvent{}, fmt.Errorf("stripe decode: %w", err)
	}
	var t appPayment.EventType
	switch wrapper.Type {
	case "checkout.session.completed":
		t = appPayment.EventCheckoutCompleted
	case "checkout.session.expired":
		t = appPayment.EventCheckoutExpired
	case "charge.refunded":
		t = appPayment.EventRefunded
	case "invoice.paid", "invoice.payment_succeeded":
		t = appPayment.EventInvoicePaid
	case "customer.subscription.deleted":
		t = appPayment.EventSubscriptionEnded
	default:
		t = appPayment.EventUnknown
	}

	// 两种主要负载：
	//  - checkout.session.* → object.client_reference_id / id / metadata
	//  - invoice.paid       → object.subscription / customer / metadata（若在订阅上）
	var sess struct {
		ID              string            `json:"id"`
		ClientRefID     string            `json:"client_reference_id"`
		PaymentIntent   string            `json:"payment_intent"`
		Subscription   string            `json:"subscription"`
		Customer       string            `json:"customer"`
		Metadata       map[string]string `json:"metadata"`
		Lines          struct {
			Data []struct {
				Subscription string            `json:"subscription"`
				Metadata     map[string]string `json:"metadata"`
			} `json:"data"`
		} `json:"lines"`
	}
	_ = json.Unmarshal(wrapper.Data.Object, &sess)

	out := appPayment.WebhookEvent{
		Type:           t,
		OrderID:        sess.ClientRefID,
		RefID:          sess.ID,
		SubscriptionID: sess.Subscription,
		Meta:           map[string]any{"stripe_event": wrapper.ID, "payment_intent": sess.PaymentIntent},
	}
	// metadata.order_id / user_id / plan_code 透传
	if sess.Metadata != nil {
		if v := sess.Metadata["order_id"]; v != "" && out.OrderID == "" {
			out.OrderID = v
		}
		if v := sess.Metadata["user_id"]; v != "" {
			if n, err := strconv.ParseUint(v, 10, 64); err == nil {
				out.UserID = n
			}
		}
		if v := sess.Metadata["plan_code"]; v != "" {
			out.PlanCode = v
		}
	}
	// invoice.lines[0].subscription + metadata 兜底
	if out.SubscriptionID == "" && len(sess.Lines.Data) > 0 {
		out.SubscriptionID = sess.Lines.Data[0].Subscription
		if meta := sess.Lines.Data[0].Metadata; meta != nil {
			if v := meta["plan_code"]; v != "" && out.PlanCode == "" {
				out.PlanCode = v
			}
		}
	}
	return out, nil
}

// parseSigHeader 拆解 Stripe-Signature 头。
func parseSigHeader(h string) (ts int64, sigs []string, err error) {
	for _, part := range strings.Split(h, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "t":
			n, e := strconv.ParseInt(kv[1], 10, 64)
			if e != nil {
				return 0, nil, fmt.Errorf("stripe: bad timestamp: %w", e)
			}
			ts = n
		case "v1":
			sigs = append(sigs, kv[1])
		}
	}
	if ts == 0 || len(sigs) == 0 {
		return 0, nil, fmt.Errorf("stripe: malformed signature header")
	}
	return
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "...(truncated)"
}
