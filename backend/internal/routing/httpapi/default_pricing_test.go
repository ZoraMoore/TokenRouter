package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

func TestDefaultPricingManualUpdateIsSeparateFromQuery(t *testing.T) {
	updates, reads := 0, 0
	var updateErr error
	catalog := &routing.PricingCatalog{
		List: func(_ routing.DefaultPricingQuery) routing.DefaultPricingPage {
			reads++
			return routing.DefaultPricingPage{}
		},
		Update: func() error {
			updates++
			return updateErr
		},
	}
	router := gin.New()
	RegisterPricingRoutes(router.Group("/api/v1/admin"), NewPricingHandler(nil, catalog))
	query := httptest.NewRecorder()
	router.ServeHTTP(query, httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing/defaults", nil))
	require.Equal(t, http.StatusOK, query.Code)
	require.Equal(t, 1, reads)
	require.Zero(t, updates, "普通刷新只能读取已加载目录")
	update := httptest.NewRecorder()
	router.ServeHTTP(update, httptest.NewRequest(http.MethodPost, "/api/v1/admin/pricing/defaults/update", nil))
	require.Equal(t, http.StatusOK, update.Code)
	require.True(t, gjson.Get(update.Body.String(), "data.updated").Bool())
	require.Equal(t, 1, updates)
	require.Equal(t, 1, reads)
	updateErr = errors.New("remote unavailable")
	failed := httptest.NewRecorder()
	router.ServeHTTP(failed, httptest.NewRequest(http.MethodPost, "/api/v1/admin/pricing/defaults/update", nil))
	require.Equal(t, http.StatusBadGateway, failed.Code)
	catalog.Update = nil
	unavailable := httptest.NewRecorder()
	router.ServeHTTP(unavailable, httptest.NewRequest(http.MethodPost, "/api/v1/admin/pricing/defaults/update", nil))
	require.Equal(t, http.StatusServiceUnavailable, unavailable.Code)
}

func TestDefaultPricingFiltersPaginationAndZero(t *testing.T) {
	zero := 0.0
	index := routing.NewDefaultPricingIndex([]routing.DefaultPricingModel{
		{Model: "z", Platform: "openai", BillingMode: "token"},
		{Model: "a", Platform: "openai", BillingMode: "token"},
		{Model: "a-image", Platform: "grok", BillingMode: "image"},
	})
	prices := map[string]pricing.DefaultModelPrice{
		"z":       {Model: "z", Platform: "openai", BillingMode: "token", PriceStatus: "priced", Prices: []pricing.DefaultPriceValue{{Key: "input", Value: &zero, Unit: "USD/MTok"}}},
		"a":       {Model: "a", Platform: "openai", BillingMode: "token", PriceStatus: "unpriced"},
		"a-image": {Model: "a-image", Platform: "grok", BillingMode: "image", PriceStatus: "priced"},
	}
	catalog := &routing.PricingCatalog{List: func(query routing.DefaultPricingQuery) routing.DefaultPricingPage {
		page := index.Page(query, func(model, _, _ string) pricing.DefaultModelPrice {
			return prices[model]
		})
		page.Version = "catalog-hash"
		page.LastError = "sync failed"
		page.UpdatedAt = time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC)
		return page
	}}
	router := gin.New()
	RegisterPricingRoutes(router.Group("/api/v1/admin"), NewPricingHandler(nil, catalog))
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing/defaults?platform=openai&billing_mode=token&page=2&page_size=1", nil))
	require.Equal(t, http.StatusOK, response.Code)
	require.Equal(t, int64(2), gjson.Get(response.Body.String(), "data.total").Int())
	require.Equal(t, "z", gjson.Get(response.Body.String(), "data.items.0.model").String())
	require.True(t, gjson.Get(response.Body.String(), "data.items.0.prices.0.value").Exists())
	require.Equal(t, 0.0, gjson.Get(response.Body.String(), "data.items.0.prices.0.value").Float())
	require.JSONEq(t, `["grok","openai"]`, gjson.Get(response.Body.String(), "data.platforms").Raw)
	require.Equal(t, "catalog-hash", gjson.Get(response.Body.String(), "data.version").String())
	require.Equal(t, "sync failed", gjson.Get(response.Body.String(), "data.last_error").String())
	require.Equal(t, "2026-11-20T00:00:00Z", gjson.Get(response.Body.String(), "data.last_updated").String())
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing/defaults?search=A-IMAGE", nil))
	require.Equal(t, int64(1), gjson.Get(response.Body.String(), "data.total").Int())
	response = httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/admin/pricing/defaults?platform=openai&page=10&limit=1", nil))
	require.Equal(t, int64(2), gjson.Get(response.Body.String(), "data.total").Int())
	require.JSONEq(t, `[]`, gjson.Get(response.Body.String(), "data.items").Raw)
}
