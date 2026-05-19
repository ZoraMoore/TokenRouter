package provider

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/payment"
)

const (
	bepusdtDefaultFiat        = payment.DefaultPaymentCurrency
	bepusdtDefaultTimeoutSec  = 1800
	bepusdtTradeTypeUSDTBEP20 = "usdt.bep20"
	bepusdtHTTPTimeout        = 15 * time.Second
	maxBEPUSDTResponseSize    = 1 << 20
	maxBEPUSDTErrorSummary    = 512
	bepusdtStatusPaid         = "2"
)

// BEPUSDT implements payment.Provider for a standalone BEpusdt cashier.
type BEPUSDT struct {
	instanceID string
	config     map[string]string
	httpClient *http.Client
}

// NewBEPUSDT creates a BEpusdt provider.
// Required config keys: apiBase, apiToken, notifyUrl, returnUrl.
func NewBEPUSDT(instanceID string, config map[string]string) (*BEPUSDT, error) {
	for _, key := range []string{"apiBase", "apiToken", "notifyUrl", "returnUrl"} {
		if strings.TrimSpace(config[key]) == "" {
			return nil, fmt.Errorf("bepusdt config missing required key: %s", key)
		}
	}
	cfg := make(map[string]string, len(config))
	for k, v := range config {
		cfg[k] = strings.TrimSpace(v)
	}
	cfg["apiBase"] = normalizeBEPUSDTAPIBase(cfg["apiBase"])
	if cfg["fiat"] == "" {
		cfg["fiat"] = bepusdtDefaultFiat
	}
	if cfg["tradeType"] == "" {
		cfg["tradeType"] = bepusdtTradeTypeUSDTBEP20
	}
	if cfg["tradeType"] != bepusdtTradeTypeUSDTBEP20 {
		return nil, fmt.Errorf("bepusdt tradeType must be %s", bepusdtTradeTypeUSDTBEP20)
	}
	return &BEPUSDT{
		instanceID: instanceID,
		config:     cfg,
		httpClient: &http.Client{Timeout: bepusdtHTTPTimeout},
	}, nil
}

func normalizeBEPUSDTAPIBase(apiBase string) string {
	base := strings.TrimSpace(apiBase)
	if base == "" {
		return ""
	}
	if parsed, err := url.Parse(base); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.RawQuery = ""
		parsed.Fragment = ""
		parsed.RawPath = ""
		parsed.Path = strings.TrimRight(strings.TrimSuffix(parsed.Path, "/api/v1"), "/")
		return strings.TrimRight(parsed.String(), "/")
	}
	return strings.TrimRight(strings.TrimSuffix(base, "/api/v1"), "/")
}

func (b *BEPUSDT) Name() string        { return "BEpusdt" }
func (b *BEPUSDT) ProviderKey() string { return payment.TypeBEPUSDT }
func (b *BEPUSDT) SupportedTypes() []payment.PaymentType {
	return []payment.PaymentType{payment.TypeUSDTBEP20}
}

func (b *BEPUSDT) MerchantIdentityMetadata() map[string]string {
	return map[string]string{
		"trade_type": bepusdtTradeTypeUSDTBEP20,
		"currency":   strings.ToUpper(strings.TrimSpace(b.config["fiat"])),
	}
}

func (b *BEPUSDT) CreatePayment(ctx context.Context, req payment.CreatePaymentRequest) (*payment.CreatePaymentResponse, error) {
	timeoutSec := bepusdtDefaultTimeoutSec
	if raw := strings.TrimSpace(b.config["timeout"]); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed >= 120 {
			timeoutSec = parsed
		}
	}
	amount, err := strconv.ParseFloat(strings.TrimSpace(req.Amount), 64)
	if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) {
		return nil, fmt.Errorf("bepusdt invalid amount: %s", req.Amount)
	}
	params := map[string]any{
		"order_id":     req.OrderID,
		"amount":       amount,
		"fiat":         strings.ToUpper(strings.TrimSpace(b.config["fiat"])),
		"trade_type":   bepusdtTradeTypeUSDTBEP20,
		"name":         req.Subject,
		"notify_url":   firstNonEmpty(req.NotifyURL, b.config["notifyUrl"]),
		"redirect_url": firstNonEmpty(req.ReturnURL, b.config["returnUrl"]),
		"timeout":      timeoutSec,
	}
	params["signature"] = bepusdtSignValues(params, b.config["apiToken"])

	body, err := b.postJSON(ctx, b.config["apiBase"]+"/api/v1/order/create-transaction", params)
	if err != nil {
		return nil, fmt.Errorf("bepusdt create: %w", err)
	}

	var resp struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
		Data       struct {
			TradeID       string `json:"trade_id"`
			OrderID       string `json:"order_id"`
			Amount        string `json:"amount"`
			ActualAmount  string `json:"actual_amount"`
			Token         string `json:"token"`
			PaymentURL    string `json:"payment_url"`
			Status        any    `json:"status"`
			ExpirationSec int    `json:"expiration_time"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("bepusdt parse create: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bepusdt error: %s", resp.Message)
	}
	if strings.TrimSpace(resp.Data.TradeID) == "" || strings.TrimSpace(resp.Data.PaymentURL) == "" {
		return nil, fmt.Errorf("bepusdt create missing trade_id or payment_url")
	}
	return &payment.CreatePaymentResponse{
		TradeNo:  resp.Data.TradeID,
		PayURL:   resp.Data.PaymentURL,
		QRCode:   resp.Data.Token,
		Currency: strings.ToUpper(strings.TrimSpace(b.config["fiat"])),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (b *BEPUSDT) QueryOrder(context.Context, string) (*payment.QueryOrderResponse, error) {
	return nil, fmt.Errorf("bepusdt query order is not supported")
}

func (b *BEPUSDT) CancelPayment(ctx context.Context, tradeNo string) error {
	tradeNo = strings.TrimSpace(tradeNo)
	if tradeNo == "" {
		return fmt.Errorf("bepusdt cancel missing trade no")
	}

	params := map[string]any{
		"trade_id": tradeNo,
	}
	params["signature"] = bepusdtSignValues(params, b.config["apiToken"])

	body, err := b.postJSON(ctx, b.config["apiBase"]+"/api/v1/order/cancel-transaction", params)
	if err != nil {
		return fmt.Errorf("bepusdt cancel: %w", err)
	}

	var resp struct {
		StatusCode int    `json:"status_code"`
		Message    string `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("bepusdt parse cancel: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bepusdt cancel error: %s", resp.Message)
	}

	return nil
}

func (b *BEPUSDT) VerifyNotification(_ context.Context, rawBody string, _ map[string]string) (*payment.PaymentNotification, error) {
	params, err := bepusdtNotificationParams(rawBody)
	if err != nil {
		return nil, err
	}
	signature := strings.TrimSpace(params["signature"])
	if signature == "" {
		return nil, fmt.Errorf("bepusdt notify missing signature")
	}
	expected := bepusdtSign(params, b.config["apiToken"])
	if subtle.ConstantTimeCompare([]byte(strings.ToLower(signature)), []byte(expected)) != 1 {
		return nil, fmt.Errorf("bepusdt notify invalid signature")
	}

	status := payment.ProviderStatusFailed
	if strings.TrimSpace(params["status"]) == bepusdtStatusPaid {
		status = payment.ProviderStatusSuccess
	}
	amount, _ := strconv.ParseFloat(strings.TrimSpace(params["amount"]), 64)
	metadata := b.MerchantIdentityMetadata()
	if actualAmount := strings.TrimSpace(params["actual_amount"]); actualAmount != "" {
		metadata["actual_amount"] = actualAmount
	}
	if txID := strings.TrimSpace(params["block_transaction_id"]); txID != "" {
		metadata["block_transaction_id"] = txID
	}
	return &payment.PaymentNotification{
		TradeNo:  strings.TrimSpace(params["trade_id"]),
		OrderID:  strings.TrimSpace(params["order_id"]),
		Amount:   amount,
		Status:   status,
		RawData:  rawBody,
		Metadata: metadata,
	}, nil
}

func (b *BEPUSDT) Refund(context.Context, payment.RefundRequest) (*payment.RefundResponse, error) {
	return nil, fmt.Errorf("bepusdt refund is not supported")
}

func (b *BEPUSDT) postJSON(ctx context.Context, endpoint string, payload map[string]any) ([]byte, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBEPUSDTResponseSize))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, summarizeBEPUSDTResponse(body))
	}
	return body, nil
}

func summarizeBEPUSDTResponse(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > maxBEPUSDTErrorSummary {
		return s[:maxBEPUSDTErrorSummary] + "...(truncated)"
	}
	return s
}

func bepusdtNotificationParams(rawBody string) (map[string]string, error) {
	rawBody = strings.TrimSpace(rawBody)
	if rawBody == "" {
		return nil, fmt.Errorf("bepusdt notify body is empty")
	}
	if strings.HasPrefix(rawBody, "{") {
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(rawBody), &raw); err != nil {
			return nil, fmt.Errorf("bepusdt parse notify json: %w", err)
		}
		params := make(map[string]string, len(raw))
		for key, value := range raw {
			params[key] = bepusdtRawJSONValue(value)
		}
		return params, nil
	}
	values, err := url.ParseQuery(rawBody)
	if err != nil {
		return nil, fmt.Errorf("bepusdt parse notify form: %w", err)
	}
	params := make(map[string]string, len(values))
	for key := range values {
		params[key] = values.Get(key)
	}
	return params, nil
}

func bepusdtRawJSONValue(raw json.RawMessage) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return ""
	}
	var str string
	if err := json.Unmarshal(raw, &str); err == nil {
		return str
	}
	return trimmed
}

func bepusdtSign(params map[string]string, token string) string {
	values := make(map[string]any, len(params))
	for key, value := range params {
		values[key] = value
	}
	return bepusdtSignValues(values, token)
}

func bepusdtSignValues(params map[string]any, token string) string {
	keys := make([]string, 0, len(params))
	for key, value := range params {
		if key == "signature" || value == nil || strings.TrimSpace(fmt.Sprintf("%v", value)) == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, key+"="+fmt.Sprintf("%v", params[key]))
	}
	sum := md5.Sum([]byte(strings.Join(parts, "&") + token))
	return hex.EncodeToString(sum[:])
}
