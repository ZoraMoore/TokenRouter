package httpapi

import (
	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/payment"
	response "github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

type rechargeCouponQuoteRequest struct {
	Amount      float64 `json:"amount" binding:"required,gt=0"`
	CouponID    int64   `json:"coupon_id" binding:"required,gt=0"`
	PaymentType string  `json:"payment_type" binding:"required,max=30"`
}

// ListRechargeCoupons 分页返回登录用户领取的券。
func (h *PaymentHandler) ListRechargeCoupons(c *gin.Context) {
	subject, ok := UserRequireAuth(c)
	if !ok {
		return
	}
	page, size := response.ParsePagination(c)
	size = min(size, 100)
	coupons, total, err := h.paymentService.ListRechargeCoupons(c.Request.Context(), subject.UserID, c.Query("available") == "true", page, size)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, coupons, total, page, size)
}

// QuoteRechargeCoupon 按登录用户、充值额及所选方式计算满减。
func (h *PaymentHandler) QuoteRechargeCoupon(c *gin.Context) {
	subject, ok := UserRequireAuth(c)
	if !ok {
		return
	}
	var req rechargeCouponQuoteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "invalid recharge coupon quote request")
		return
	}
	quote, err := h.paymentService.QuoteRechargeCoupon(c.Request.Context(), payment.CreateOrderRequest{
		UserID: subject.UserID, Amount: req.Amount, CouponID: req.CouponID, PaymentType: req.PaymentType,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, quote)
}
