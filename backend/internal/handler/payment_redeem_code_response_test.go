package handler

import (
	"testing"
	"time"

	dbent "github.com/TokenFlux/TokenRouter/ent"
	"github.com/TokenFlux/TokenRouter/internal/payment"
	"github.com/TokenFlux/TokenRouter/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSanitizePaymentOrderForResponseExposesRedeemCodeForCompletedDeliveryOrder(t *testing.T) {
	providerKey := payment.TypeBEPUSDT
	result := sanitizePaymentOrderForResponse(&dbent.PaymentOrder{
		ID:           99,
		UserID:       1,
		Amount:       20,
		PayAmount:    20,
		PaymentType:  payment.TypeUSDTBEP20,
		ProviderKey:  &providerKey,
		RechargeCode: "PAY-CODE-999",
		Status:       service.OrderStatusCompleted,
		OrderType:    payment.OrderTypeBalance,
		ExpiresAt:    time.Now().Add(time.Hour),
		CreatedAt:    time.Now(),
	})

	require.NotNil(t, result)
	require.Equal(t, "PAY-CODE-999", result.RedeemCode)
}

func TestBuildPublicOrderResultDoesNotExposeRedeemCodeWithoutResumeToken(t *testing.T) {
	providerKey := payment.TypeBEPUSDT
	order := &dbent.PaymentOrder{
		ID:           99,
		Amount:       20,
		PayAmount:    20,
		PaymentType:  payment.TypeUSDTBEP20,
		ProviderKey:  &providerKey,
		RechargeCode: "PAY-CODE-999",
		Status:       service.OrderStatusCompleted,
		OrderType:    payment.OrderTypeBalance,
		ExpiresAt:    time.Now().Add(time.Hour),
		CreatedAt:    time.Now(),
	}

	require.Empty(t, buildPublicOrderResult(order).RedeemCode)
	require.Equal(t, "PAY-CODE-999", buildPublicOrderResultWithRedeemCode(order, true).RedeemCode)
}
