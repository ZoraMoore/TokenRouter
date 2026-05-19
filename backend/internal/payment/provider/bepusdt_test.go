package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/TokenFlux/TokenRouter/internal/payment"
)

func TestBEPUSDTCreatePaymentUsesBEP20TradeType(t *testing.T) {
	const token = "test-token"
	var captured map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/order/create-transaction" {
			t.Fatalf("path = %s, want /api/v1/order/create-transaction", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if captured["trade_type"] != "usdt.bep20" {
			t.Fatalf("trade_type = %q, want usdt.bep20", captured["trade_type"])
		}
		if captured["signature"] != bepusdtSign(captured, token) {
			t.Fatalf("signature mismatch")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status_code": http.StatusOK,
			"message":     "ok",
			"data": map[string]any{
				"trade_id":    "trade-1",
				"order_id":    captured["order_id"],
				"amount":      captured["amount"],
				"payment_url": "https://pay.example.test/order/trade-1",
				"token":       "qr-token",
			},
		})
	}))
	defer server.Close()

	provider, err := NewBEPUSDT("1", map[string]string{
		"apiBase":   server.URL,
		"apiToken":  token,
		"notifyUrl": "https://tokenrouter.example/api/v1/payment/webhook/bepusdt",
		"returnUrl": "https://tokenrouter.example/payment/result",
		"fiat":      "USD",
		"timeout":   "600",
	})
	if err != nil {
		t.Fatalf("NewBEPUSDT: %v", err)
	}

	resp, err := provider.CreatePayment(context.Background(), payment.CreatePaymentRequest{
		OrderID:   "order-1",
		Amount:    "12.34",
		Subject:   "TokenRouter 12.34",
		NotifyURL: "https://notify.example/webhook",
		ReturnURL: "https://return.example/result",
	})
	if err != nil {
		t.Fatalf("CreatePayment: %v", err)
	}
	if resp.TradeNo != "trade-1" || resp.PayURL == "" || resp.QRCode != "qr-token" || resp.Currency != "USD" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if captured["notify_url"] != "https://notify.example/webhook" || captured["redirect_url"] != "https://return.example/result" {
		t.Fatalf("callback urls not forwarded: %+v", captured)
	}
}

func TestBEPUSDTVerifyNotificationPaid(t *testing.T) {
	const token = "test-token"
	provider, err := NewBEPUSDT("1", map[string]string{
		"apiBase":   "https://pay.example.test",
		"apiToken":  token,
		"notifyUrl": "https://tokenrouter.example/api/v1/payment/webhook/bepusdt",
		"returnUrl": "https://tokenrouter.example/payment/result",
	})
	if err != nil {
		t.Fatalf("NewBEPUSDT: %v", err)
	}
	params := map[string]string{
		"order_id":             "order-1",
		"trade_id":             "trade-1",
		"amount":               "12.34",
		"actual_amount":        "1.23",
		"block_transaction_id": "0xabc",
		"status":               "2",
	}
	params["signature"] = bepusdtSign(params, token)
	raw, _ := json.Marshal(params)

	notification, err := provider.VerifyNotification(context.Background(), string(raw), nil)
	if err != nil {
		t.Fatalf("VerifyNotification: %v", err)
	}
	if notification.Status != payment.ProviderStatusSuccess {
		t.Fatalf("status = %q, want success", notification.Status)
	}
	if notification.OrderID != "order-1" || notification.TradeNo != "trade-1" || notification.Amount != 12.34 {
		t.Fatalf("unexpected notification: %+v", notification)
	}
	if notification.Metadata["trade_type"] != "usdt.bep20" || notification.Metadata["actual_amount"] != "1.23" {
		t.Fatalf("unexpected metadata: %+v", notification.Metadata)
	}
}

func TestBEPUSDTVerifyNotificationRejectsBadSignature(t *testing.T) {
	provider, err := NewBEPUSDT("1", map[string]string{
		"apiBase":   "https://pay.example.test",
		"apiToken":  "test-token",
		"notifyUrl": "https://tokenrouter.example/api/v1/payment/webhook/bepusdt",
		"returnUrl": "https://tokenrouter.example/payment/result",
	})
	if err != nil {
		t.Fatalf("NewBEPUSDT: %v", err)
	}
	_, err = provider.VerifyNotification(context.Background(), `{"order_id":"order-1","trade_id":"trade-1","amount":"12.34","status":"2","signature":"bad"}`, nil)
	if err == nil {
		t.Fatalf("VerifyNotification expected signature error")
	}
}
