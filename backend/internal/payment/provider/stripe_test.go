//go:build unit

package provider

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/payment"
	stripe "github.com/stripe/stripe-go/v85"
)

func TestBuildStripeInvoiceCreateParamsUsesHostedInvoiceMode(t *testing.T) {
	t.Parallel()

	expiresAt := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second)
	params := buildStripeInvoiceCreateParams("cus_123", payment.CreatePaymentRequest{
		OrderID:   "sub2_order_123",
		Subject:   "TokenRouter Balance",
		ExpiresAt: expiresAt,
	}, []string{"card", "link", "wechat_pay"}, "42")

	if params.Customer == nil || *params.Customer != "cus_123" {
		t.Fatalf("customer = %#v, want cus_123", params.Customer)
	}
	if params.CollectionMethod == nil || *params.CollectionMethod != string(stripe.InvoiceCollectionMethodSendInvoice) {
		t.Fatalf("collection_method = %#v, want send_invoice", params.CollectionMethod)
	}
	if params.DueDate == nil || *params.DueDate != expiresAt.Unix() {
		t.Fatalf("due_date = %#v, want %d", params.DueDate, expiresAt.Unix())
	}
	if params.DaysUntilDue != nil {
		t.Fatalf("days_until_due should be empty when due_date is set")
	}
	if params.PaymentSettings == nil || len(params.PaymentSettings.PaymentMethodTypes) != 3 {
		t.Fatalf("payment method types = %#v", params.PaymentSettings)
	}
	if got := *params.PaymentSettings.PaymentMethodTypes[2]; got != "wechat_pay" {
		t.Fatalf("payment method type[2] = %q, want wechat_pay", got)
	}
	if params.Metadata["orderId"] != "sub2_order_123" {
		t.Fatalf("metadata orderId = %q", params.Metadata["orderId"])
	}
	if params.Metadata["providerInstanceId"] != "42" {
		t.Fatalf("metadata providerInstanceId = %q", params.Metadata["providerInstanceId"])
	}
}

func TestBuildStripeInvoiceCreateParamsOmitsPaymentSettingsWhenMethodsEmpty(t *testing.T) {
	t.Parallel()

	params := buildStripeInvoiceCreateParams("cus_123", payment.CreatePaymentRequest{
		OrderID: "sub2_order_123",
		Subject: "TokenRouter Balance",
	}, nil, "42")

	if params.PaymentSettings != nil {
		t.Fatalf("payment settings = %#v, want nil", params.PaymentSettings)
	}
}

func TestStripeInvoicePaymentMethodTypesMapsConfiguredSubMethods(t *testing.T) {
	t.Parallel()

	got := stripeInvoicePaymentMethodTypes("card,alipay,wxpay,link,stripe,unknown,alipay")
	want := []string{"card", "alipay", "wechat_pay", "link"}
	if len(got) != len(want) {
		t.Fatalf("payment method types = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("payment method types = %#v, want %#v", got, want)
		}
	}
}

func TestBuildStripeInvoiceCreateParamsUsesConfiguredSubMethods(t *testing.T) {
	t.Parallel()

	req := payment.CreatePaymentRequest{
		OrderID:            "sub2_order_123",
		Subject:            "TokenRouter Balance",
		InstanceSubMethods: "card,alipay,wxpay,link",
	}
	params := buildStripeInvoiceCreateParams("cus_123", req, stripeInvoicePaymentMethodTypes(req.InstanceSubMethods), "42")

	if params.PaymentSettings == nil || len(params.PaymentSettings.PaymentMethodTypes) != 4 {
		t.Fatalf("payment method types = %#v, want 4 methods", params.PaymentSettings)
	}
	got := make([]string, 0, len(params.PaymentSettings.PaymentMethodTypes))
	for _, method := range params.PaymentSettings.PaymentMethodTypes {
		got = append(got, *method)
	}
	want := []string{"card", "alipay", "wechat_pay", "link"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("payment method types = %#v, want %#v", got, want)
		}
	}
}

func TestBuildStripeInvoiceCreateParamsFallsBackToOneDayDue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		req  payment.CreatePaymentRequest
	}{
		{name: "missing expiry", req: payment.CreatePaymentRequest{}},
		{name: "past expiry", req: payment.CreatePaymentRequest{ExpiresAt: time.Now().Add(-time.Minute)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			params := buildStripeInvoiceCreateParams("cus_123", tt.req, []string{"card"}, "")
			if params.DueDate != nil {
				t.Fatalf("due_date should be empty without a usable order expiry")
			}
			if params.DaysUntilDue == nil || *params.DaysUntilDue != 1 {
				t.Fatalf("days_until_due = %#v, want 1", params.DaysUntilDue)
			}
		})
	}
}

func TestStripePaymentIntentIDFromClientSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		secret string
		want   string
	}{
		{
			name:   "extracts payment intent id",
			secret: "pi_123_secret_abc",
			want:   "pi_123",
		},
		{
			name:   "trims whitespace before extracting",
			secret: "  pi_456_secret_def  ",
			want:   "pi_456",
		},
		{
			name:   "rejects non payment intent secret",
			secret: "seti_123_secret_abc",
			want:   "",
		},
		{
			name:   "requires secret marker",
			secret: "pi_123",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := stripePaymentIntentIDFromClientSecret(tt.secret); got != tt.want {
				t.Fatalf("payment intent id = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripeInvoiceTradeNo(t *testing.T) {
	t.Parallel()

	invoiceWithPayment := &stripe.Invoice{
		ID: "in_with_payment",
		Payments: &stripe.InvoicePaymentList{Data: []*stripe.InvoicePayment{
			{
				Payment: &stripe.InvoicePaymentPayment{
					PaymentIntent: &stripe.PaymentIntent{ID: "pi_from_payment"},
				},
			},
		}},
	}
	if got := stripeInvoiceTradeNo(invoiceWithPayment, "pi_123_secret_abc"); got != "pi_from_payment" {
		t.Fatalf("trade no = %q, want expanded payment intent", got)
	}

	invoiceWithoutPayment := &stripe.Invoice{ID: "in_without_payment"}
	if got := stripeInvoiceTradeNo(invoiceWithoutPayment, "pi_123_secret_abc"); got != "pi_123" {
		t.Fatalf("trade no = %q, want client secret payment intent", got)
	}

	if got := stripeInvoiceTradeNo(invoiceWithoutPayment, ""); got != "in_without_payment" {
		t.Fatalf("trade no = %q, want invoice id fallback", got)
	}
}

func TestParseStripeInvoiceUsesInvoicePaymentIntent(t *testing.T) {
	t.Parallel()

	rawBody := `{"id":"evt_invoice_paid"}`
	invoiceRaw := stripeInvoiceEventRaw(t, map[string]any{
		"id":                 "in_123",
		"object":             "invoice",
		"amount_paid":        1234,
		"amount_due":         1234,
		"status":             "paid",
		"hosted_invoice_url": "https://stripe.example/invoice/in_123",
		"invoice_pdf":        "https://stripe.example/invoice/in_123.pdf",
		"metadata": map[string]string{
			"orderId": "sub2_order_123",
		},
		"payments": map[string]any{
			"object": "list",
			"data": []any{
				map[string]any{
					"id":     "inpay_123",
					"object": "invoice_payment",
					"payment": map[string]any{
						"type": "payment_intent",
						"payment_intent": map[string]any{
							"id":     "pi_123",
							"object": "payment_intent",
						},
					},
				},
			},
		},
	})

	notification, err := parseStripeInvoice(&stripe.Event{
		Data: &stripe.EventData{Raw: invoiceRaw},
	}, payment.ProviderStatusSuccess, rawBody)
	if err != nil {
		t.Fatalf("parse invoice: %v", err)
	}

	if notification.TradeNo != "pi_123" {
		t.Fatalf("trade no = %q, want %q", notification.TradeNo, "pi_123")
	}
	if notification.OrderID != "sub2_order_123" {
		t.Fatalf("order id = %q, want %q", notification.OrderID, "sub2_order_123")
	}
	if notification.Amount != 12.34 {
		t.Fatalf("amount = %.2f, want 12.34", notification.Amount)
	}
	if notification.Status != payment.ProviderStatusSuccess {
		t.Fatalf("status = %q, want %q", notification.Status, payment.ProviderStatusSuccess)
	}
	if notification.RawData != rawBody {
		t.Fatalf("raw body = %q, want %q", notification.RawData, rawBody)
	}
	if notification.Metadata["invoice_id"] != "in_123" {
		t.Fatalf("invoice_id metadata = %q", notification.Metadata["invoice_id"])
	}
	if notification.Metadata["invoice_status"] != "paid" {
		t.Fatalf("invoice_status metadata = %q", notification.Metadata["invoice_status"])
	}
	if notification.Metadata["invoice_url"] != "https://stripe.example/invoice/in_123" {
		t.Fatalf("invoice_url metadata = %q", notification.Metadata["invoice_url"])
	}
	if notification.Metadata["invoice_pdf"] != "https://stripe.example/invoice/in_123.pdf" {
		t.Fatalf("invoice_pdf metadata = %q", notification.Metadata["invoice_pdf"])
	}
}

func TestParseStripeInvoiceFallsBackToInvoiceIDAndAmountDue(t *testing.T) {
	t.Parallel()

	invoiceRaw := stripeInvoiceEventRaw(t, map[string]any{
		"id":          "in_due",
		"object":      "invoice",
		"amount_paid": 0,
		"amount_due":  8800,
		"status":      "open",
		"metadata": map[string]string{
			"orderId": "sub2_due",
		},
	})

	notification, err := parseStripeInvoice(&stripe.Event{
		Data: &stripe.EventData{Raw: invoiceRaw},
	}, payment.ProviderStatusFailed, "{}")
	if err != nil {
		t.Fatalf("parse invoice: %v", err)
	}

	if notification.TradeNo != "in_due" {
		t.Fatalf("trade no = %q, want %q", notification.TradeNo, "in_due")
	}
	if notification.Amount != 88 {
		t.Fatalf("amount = %.2f, want 88.00", notification.Amount)
	}
	if notification.Status != payment.ProviderStatusFailed {
		t.Fatalf("status = %q, want %q", notification.Status, payment.ProviderStatusFailed)
	}
}

func TestStripeInvoiceDocumentResponse(t *testing.T) {
	t.Parallel()

	withHosted := stripeInvoiceDocumentResponse(&stripe.Invoice{
		ID:               "in_hosted",
		Status:           stripe.InvoiceStatusPaid,
		HostedInvoiceURL: " https://stripe.example/invoice/hosted ",
		InvoicePDF:       "https://stripe.example/invoice/hosted.pdf",
	})
	if withHosted.Type != "invoice" {
		t.Fatalf("type = %q, want invoice", withHosted.Type)
	}
	if withHosted.URL != "https://stripe.example/invoice/hosted" {
		t.Fatalf("url = %q, want hosted invoice url", withHosted.URL)
	}
	if withHosted.HostedInvoiceURL != " https://stripe.example/invoice/hosted " {
		t.Fatalf("hosted invoice url should preserve provider value")
	}
	if withHosted.InvoicePDF != "https://stripe.example/invoice/hosted.pdf" {
		t.Fatalf("invoice pdf = %q", withHosted.InvoicePDF)
	}
	if withHosted.InvoiceID != "in_hosted" {
		t.Fatalf("invoice id = %q", withHosted.InvoiceID)
	}
	if withHosted.InvoiceStatus != string(stripe.InvoiceStatusPaid) {
		t.Fatalf("invoice status = %q", withHosted.InvoiceStatus)
	}

	withPDF := stripeInvoiceDocumentResponse(&stripe.Invoice{
		ID:         "in_pdf",
		InvoicePDF: " https://stripe.example/invoice/pdf.pdf ",
	})
	if withPDF.URL != "https://stripe.example/invoice/pdf.pdf" {
		t.Fatalf("url = %q, want pdf fallback", withPDF.URL)
	}

	empty := stripeInvoiceDocumentResponse(nil)
	if empty == nil || empty.Type != "invoice" {
		t.Fatalf("nil invoice response = %#v", empty)
	}
}

func stripeInvoiceEventRaw(t *testing.T, invoice map[string]any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(invoice)
	if err != nil {
		t.Fatalf("marshal invoice fixture: %v", err)
	}
	return raw
}
