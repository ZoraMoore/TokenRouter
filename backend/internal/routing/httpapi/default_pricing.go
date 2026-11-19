package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// UpdateDefaultPricing 只在管理员明确更新时重新加载目录，普通查询不触发同步。
// @project-doc docs/interfaces/model_catalog_and_marketplace.md#gateway_default_pricing
func (h *PricingHandler) UpdateDefaultPricing(c *gin.Context) {
	if h.catalog == nil || h.catalog.Update == nil {
		httpx.Error(c, http.StatusServiceUnavailable, "pricing catalog updater is unavailable")
		return
	}
	if err := h.catalog.Update(); err != nil {
		slog.Warn("failed to update pricing catalog", "error", err)
		httpx.Error(c, http.StatusBadGateway, "failed to update pricing catalog")
		return
	}
	httpx.Success(c, gin.H{"updated": true})
}

// ListDefaultPricing 查询网关内存中已加载的默认价格。
// @project-doc docs/interfaces/model_catalog_and_marketplace.md#gateway_default_pricing
func (h *PricingHandler) ListDefaultPricing(c *gin.Context) {
	if h.catalog == nil || h.catalog.List == nil {
		httpx.Error(c, http.StatusServiceUnavailable, "pricing catalog is unavailable")
		return
	}
	page, pageSize := httpx.ParsePagination(c)
	httpx.Success(c, h.catalog.List(routing.DefaultPricingQuery{
		Search:      c.Query("search"),
		Platform:    c.Query("platform"),
		BillingMode: c.Query("billing_mode"),
		Page:        page,
		PageSize:    pageSize,
	}))
}
