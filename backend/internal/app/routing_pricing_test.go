package app

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/billing"
	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
	billingtestkit "github.com/TokenFlux/TokenRouter/internal/billing/testkit"
	"github.com/TokenFlux/TokenRouter/internal/modelcatalog/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	routinghttp "github.com/TokenFlux/TokenRouter/internal/routing/httpapi"
)

const (
	defaultPricingListFixture = `{"providers":{
		"openai":{"models":{"fixture-priced":{"cost":{"input":0,"output":2}},"fixture-no-price":{"name":"No price"}}},
		"openrouter":{"models":{"fixture-priced":{"cost":{"input":5,"output":6}}}},
		"xai":{"models":{"fixture-video":{"name":"Video"}}},
		"moonshot":{"models":{"fixture-moonshot":{"cost":{"input":2,"output":3}}}}
	}}`
	defaultPricingSupplement = `{
		"fixture-priced":{"time_pricing":{"timezone":"UTC","periods":[{"start_time":"09:00","end_time":"10:00","multiplier":2}]}},
		"fixture-picture":{"provider":"xai","output_cost_per_image":0,"mode":"image_generation"},
		"fixture-video":{"provider":"xai","video_prices":{"480p":0.05}}
	}`
)

type defaultPricingListResponse struct {
	Data struct {
		Total     int                         `json:"total"`
		Items     []pricing.DefaultModelPrice `json:"items"`
		Version   string                      `json:"version"`
		UpdatedAt time.Time                   `json:"last_updated"`
		LastError string                      `json:"last_error"`
		Platforms []string                    `json:"platforms"`
	} `json:"data"`
}

// TestDefaultPricingPageQuotesOnlyPage 用完整目录检查翻页和超页时的查价次数。
func TestDefaultPricingPageQuotesOnlyPage(t *testing.T) {
	router, lookups := setupDefaultPricingListForTest(t)
	readDefaultPricingList(t, router, "")
	for _, query := range []struct {
		params string
		want   int
	}{
		{"?page=2&page_size=20", 20},
		{"?page=9223372036854775807&page_size=20", 0},
	} {
		*lookups = 0
		body := readDefaultPricingList(t, router, query.params)
		require.Equal(t, query.want, *lookups, query.params)
		require.Greater(t, body.Data.Total, 20)
		require.NotNil(t, body.Data.Items)
		require.Len(t, body.Data.Items, query.want)
	}
}

// TestDefaultPricingPageMatchesDirectQuotes 对照完整报价，覆盖跨平台同名、零价和媒体计费。
func TestDefaultPricingPageMatchesDirectQuotes(t *testing.T) {
	service, _, _ := newDefaultPricingFixture(t)
	var now time.Time
	var clockReads int
	options := billing.CalculatorOptions{Now: func() time.Time {
		clockReads++
		return now
	}}
	router := newDefaultPricingListRouter(billing.NewCalculator(service, options), service)
	expectedCalculator := billing.NewCalculator(service.ReadOnlySnapshot(), options)
	for _, hour := range []int{8, 9, 10} {
		now = time.Date(2026, 11, 20, hour, 59, 59, 0, time.UTC)
		clockReads = 0
		token := readDefaultPricingList(t, router, "?search=FIXTURE-PRICED&billing_mode=token")
		require.Equal(t, 1, clockReads)
		require.Equal(t, 3, token.Data.Total)
		requireDefaultPricingJSON(t, []pricing.DefaultModelPrice{
			expectedCalculator.DefaultModelPrice("fixture-priced", "openai", ""),
			expectedCalculator.DefaultModelPrice("openai/fixture-priced", "openai", ""),
			expectedCalculator.DefaultModelPrice("openrouter/fixture-priced", "openrouter", ""),
		}, token.Data.Items)
		require.Equal(t, 0.0, *token.Data.Items[0].Prices[0].Value)
		wantOutput := 2.0
		if hour == 9 {
			wantOutput = 4
		}
		require.InDelta(t, wantOutput, *token.Data.Items[0].Prices[1].Value, 1e-12)
		require.InDelta(t, wantOutput, *token.Data.Items[1].Prices[1].Value, 1e-12)
		require.Contains(t, token.Data.Platforms, "kimi")
		require.Contains(t, token.Data.Platforms, "grok")
	}
	for _, tc := range []struct {
		params, model, platform, mode, wantMode, wantStatus string
	}{
		{"?search=fixture-picture&billing_mode=image", "fixture-picture", "grok", "image_generation", "image", "priced"},
		{"?search=fixture-video&billing_mode=video&platform=grok", "fixture-video", "grok", "", "video", "priced"},
		{"?search=fixture-no-price&billing_mode=token", "fixture-no-price", "openai", "", "token", "unpriced"},
	} {
		body := readDefaultPricingList(t, router, tc.params)
		require.Positive(t, body.Data.Total, tc.params)
		requireDefaultPricingJSON(t, expectedCalculator.DefaultModelPrice(tc.model, tc.platform, tc.mode), body.Data.Items[0])
		require.Equal(t, tc.wantMode, body.Data.Items[0].BillingMode, tc.params)
		require.Equal(t, tc.wantStatus, body.Data.Items[0].PriceStatus, tc.params)
	}
}

// TestDefaultPricingPageReloadsSupplementAndReportsFailure 检查目录版本相同时的补充重载和错误状态。
func TestDefaultPricingPageReloadsSupplementAndReportsFailure(t *testing.T) {
	service, _, supplement := newDefaultPricingFixture(t)
	router := newDefaultPricingListRouter(billing.NewCalculator(service, billing.CalculatorOptions{}), service)
	const query = "?search=fixture-video"
	before := readDefaultPricingList(t, router, query)
	require.Equal(t, "video", before.Data.Items[0].BillingMode)
	replacement := `{"fixture-video":{"provider":"xai","input_cost_per_token":0.000003,"output_cost_per_token":0.000004}}`
	require.NoError(t, os.WriteFile(supplement, []byte(replacement), 0o600))
	require.NoError(t, service.ForceUpdate())
	updated := readDefaultPricingList(t, router, query)
	require.Equal(t, before.Data.Version, updated.Data.Version)
	require.True(t, before.Data.UpdatedAt.Equal(updated.Data.UpdatedAt))
	require.Equal(t, "token", updated.Data.Items[0].BillingMode)
	require.InDelta(t, 3, *updated.Data.Items[0].Prices[0].Value, 1e-12)
	require.NoError(t, os.WriteFile(supplement, []byte("{"), 0o600))
	require.Error(t, service.ForceUpdate())
	failed := readDefaultPricingList(t, router, query)
	require.Equal(t, updated.Data.Version, failed.Data.Version)
	require.Equal(t, updated.Data.UpdatedAt, failed.Data.UpdatedAt)
	require.Equal(t, updated.Data.Items, failed.Data.Items)
	require.NotEmpty(t, failed.Data.LastError)
	require.NoError(t, os.WriteFile(supplement, []byte(replacement), 0o600))
	require.NoError(t, service.ForceUpdate())
	require.Empty(t, readDefaultPricingList(t, router, query).Data.LastError)
}

// TestDefaultPricingPageDuringRefresh 检查报价在途时发布目录及并发查询后的版本。
func TestDefaultPricingPageDuringRefresh(t *testing.T) {
	service, file, _ := newDefaultPricingFixture(t)
	var block atomic.Bool
	block.Store(true)
	started, continueQuote := make(chan struct{}), make(chan struct{})
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(continueQuote) }) })
	calculator := billing.NewCalculator(service, billing.CalculatorOptions{
		Now: func() time.Time { return time.Date(2026, 11, 20, 8, 59, 59, 0, time.UTC) },
		LoadLocation: func(zone string) (*time.Location, error) {
			if block.Swap(false) {
				close(started)
				<-continueQuote
			}
			return time.LoadLocation(zone)
		},
	})
	router := newDefaultPricingListRouter(calculator, service)
	query := httptest.NewRequest(http.MethodGet, "/pricing/defaults?search=fixture-priced&page_size=1", nil)
	oldResponse := httptest.NewRecorder()
	oldDone := make(chan struct{})
	go func() {
		router.ServeHTTP(oldResponse, query)
		close(oldDone)
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("查价未进入时段计算")
	}
	replacement := strings.ReplaceAll(defaultPricingListFixture, `"output":2`, `"output":7`)
	require.NoError(t, os.WriteFile(file, []byte(replacement), 0o600))
	require.NoError(t, service.ForceUpdate())
	current := readDefaultPricingList(t, router, "?search=fixture-priced&page_size=1")
	release.Do(func() { close(continueQuote) })
	<-oldDone
	var prior defaultPricingListResponse
	require.Equal(t, http.StatusOK, oldResponse.Code)
	require.NoError(t, json.Unmarshal(oldResponse.Body.Bytes(), &prior))
	require.NotEqual(t, current.Data.Version, prior.Data.Version)
	require.InDelta(t, 7, *current.Data.Items[0].Prices[1].Value, 1e-12)
	require.InDelta(t, 2, *prior.Data.Items[0].Prices[1].Value, 1e-12)
	require.Equal(t, current, readDefaultPricingList(t, router, "?search=fixture-priced&page_size=1"))
}

// BenchmarkAdminDefaultPricing 使用随程序发布的公开目录测量首次查询、翻页和搜索。
func BenchmarkAdminDefaultPricing(b *testing.B) {
	gin.SetMode(gin.TestMode)
	service, lookups := newDefaultPricingBenchmarkCatalog(b)
	calculator := billing.NewCalculator(service, billing.CalculatorOptions{})
	for _, query := range []struct {
		name string
		path string
	}{
		{"Cold", "/pricing/defaults"},
		{"Page", "/pricing/defaults?page=2&page_size=20"},
		{"Search", "/pricing/defaults?search=llama&page_size=20"},
	} {
		b.Run(query.name, func(b *testing.B) {
			router := newDefaultPricingListRouter(calculator, service)
			request := httptest.NewRequest(http.MethodGet, query.path, nil)
			if query.name != "Cold" {
				router.ServeHTTP(httptest.NewRecorder(), request)
			}
			*lookups = 0
			b.ReportAllocs()
			for b.Loop() {
				if query.name == "Cold" {
					router = newDefaultPricingListRouter(calculator, service)
				}
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					b.Fatalf("HTTP status %d", response.Code)
				}
			}
			b.ReportMetric(float64(*lookups)/float64(b.N), "lookups/op")
		})
	}
}

func setupDefaultPricingListForTest(t *testing.T) (*gin.Engine, *int) {
	t.Helper()
	service, lookups := newDefaultPricingBenchmarkCatalog(t)
	return newDefaultPricingListRouter(billing.NewCalculator(service, billing.CalculatorOptions{}), service), lookups
}

func newDefaultPricingBenchmarkCatalog(tb testing.TB) (*provider.Service, *int) {
	tb.Helper()
	lookups := new(int)
	service := provider.NewService(provider.Options{
		DataDir: tb.TempDir(),
		ModelLookupCandidates: func() func(string) []string {
			(*lookups)++
			return pricing.BuildModelIdentityCandidates
		},
	}, nil)
	require.NoError(tb, service.Initialize())
	return service, lookups
}

func newDefaultPricingListRouter(calculator *billing.Calculator, service *provider.Service) *gin.Engine {
	router := gin.New()
	handler := routinghttp.NewPricingHandler(nil, providePricingCatalog(calculator, service))
	router.GET("/pricing/defaults", handler.ListDefaultPricing)
	return router
}

func newDefaultPricingFixture(t *testing.T) (*provider.Service, string, string) {
	t.Helper()
	dir := t.TempDir()
	catalog, supplement := filepath.Join(dir, "models_dev_catalog.json"), filepath.Join(dir, "custom.json")
	require.NoError(t, os.WriteFile(catalog, []byte(defaultPricingListFixture), 0o600))
	require.NoError(t, os.WriteFile(supplement, []byte(defaultPricingSupplement), 0o600))
	service := provider.NewService(provider.Options{DataDir: dir, FallbackFile: supplement}, nil)
	require.NoError(t, service.Initialize())
	return service, catalog, supplement
}

func readDefaultPricingList(t *testing.T, router *gin.Engine, params string) defaultPricingListResponse {
	t.Helper()
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/pricing/defaults"+params, nil))
	require.Equal(t, http.StatusOK, response.Code)
	var output defaultPricingListResponse
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &output))
	return output
}

func requireDefaultPricingJSON(t *testing.T, expected, actual any) {
	t.Helper()
	expectedJSON, err := json.Marshal(expected)
	require.NoError(t, err)
	actualJSON, err := json.Marshal(actual)
	require.NoError(t, err)
	require.JSONEq(t, string(expectedJSON), string(actualJSON))
}

// TestModelsCatalogMediaQuotes 使用实际离线目录和管理页装配，区分输出能力与计费单位。
func TestModelsCatalogMediaQuotes(t *testing.T) {
	service := provider.NewService(provider.Options{
		DataDir: t.TempDir(),
	}, nil)
	require.NoError(t, service.Initialize())
	calculator := billing.NewCalculator(service, billing.CalculatorOptions{})
	page := providePricingCatalog(calculator, service).List(routing.DefaultPricingQuery{PageSize: math.MaxInt})
	rows := map[string]pricing.DefaultModelPrice{}
	for _, row := range page.Items {
		rows[row.Model] = row
	}
	for _, model := range []string{"gemini-omni-flash-preview", "deep-research-preview-04-2026", "gpt-image-2"} {
		t.Run(model, func(t *testing.T) {
			row, found := rows[model]
			require.True(t, found)
			require.Equal(t, "token", row.BillingMode)
			require.Equal(t, "priced", row.PriceStatus)
			for _, price := range row.Prices {
				require.NotEqual(t, "USD/s", price.Unit)
				require.NotEqual(t, "USD/image", price.Unit)
			}
		})
	}
	// 检查专用媒体报价和 Gemini 图文 token 费率。
	require.Equal(t, "video", rows["grok-imagine-video"].BillingMode)
	require.Equal(t, "image", rows["gemini-3-pro-image"].BillingMode)
	values := map[string]float64{}
	for _, price := range rows["gemini-3-pro-image"].Prices {
		if price.Value != nil {
			values[price.Key] = *price.Value
		}
	}
	require.InDelta(t, 12, values["output"], 1e-12)
	require.InDelta(t, 120, values["image_output"], 1e-12)
	require.InDelta(t, 0.134, values["1K"], 1e-12)
	attrs := service.ModelAttributes("gemini-omni-flash-preview")
	require.NotNil(t, attrs.OutputModalities)
	require.Contains(t, *attrs.OutputModalities, "video")
}

func setupModelDefaultPricingRouter(billingSvc *billing.Calculator) *gin.Engine {
	router := gin.New()
	h := routinghttp.NewPricingHandler(nil, &routing.PricingCatalog{Prices: billingSvc})
	router.GET("/pricing/defaults/model", h.GetModelDefaultPricing)
	return router
}

// TestGetModelDefaultPricing_QoderMatchesOtherPlatforms 验证同一模型的默认价不受平台影响，Qoder 别名也可以读取内置价。
func TestGetModelDefaultPricing_QoderMatchesOtherPlatforms(t *testing.T) {
	router := setupModelDefaultPricingRouter(billingtestkit.Calculator(nil, nil))
	for _, model := range []string{"claude-opus-4-6", "CLAUDE-OPUS-4-6", "qwen3.8-max", "qmodel"} {
		t.Run(model, func(t *testing.T) {
			var responses []string
			for _, platform := range []string{"qoder", "anthropic"} {
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/pricing/defaults/model?platform="+platform+"&model="+model, nil))
				require.Equal(t, http.StatusOK, w.Code)
				responses = append(responses, w.Body.String())
			}
			require.JSONEq(t, responses[0], responses[1])
			if strings.Contains(strings.ToLower(model), "claude-opus") {
				require.Contains(t, responses[0], `"found":true`)
			}
		})
	}
}

func TestGetModelDefaultPricing_Fable51ReturnsCacheTTLs(t *testing.T) {
	billingSvc := billingtestkit.Calculator(nil, nil)
	router := setupModelDefaultPricingRouter(billingSvc)
	req := httptest.NewRequest(http.MethodGet, "/pricing/defaults/model?model=claude-fable-5-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	var body struct {
		Data struct {
			Found             bool     `json:"found"`
			CacheWritePrice   float64  `json:"cache_write_price"`
			CacheWrite1hPrice *float64 `json:"cache_write_1h_price"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	require.True(t, body.Data.Found)
	require.InDelta(t, 12.5e-6, body.Data.CacheWritePrice, 1e-12)
	require.NotNil(t, body.Data.CacheWrite1hPrice)
	require.InDelta(t, 20e-6, *body.Data.CacheWrite1hPrice, 1e-12)
}

func TestGetModelDefaultPricing_UnknownQoderRouteKeysRemainUnpriced(t *testing.T) {
	billingSvc := billingtestkit.Calculator(nil, nil)
	router := setupModelDefaultPricingRouter(billingSvc)

	for _, model := range []string{"qmodel", "qmodel_38max", "ultimate", "q35model", "gmodel"} {
		req := httptest.NewRequest(http.MethodGet, "/pricing/defaults/model?platform=qoder&model="+model, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)

		var body struct {
			Data struct {
				Found      bool    `json:"found"`
				InputPrice float64 `json:"input_price"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))

		require.False(t, body.Data.Found, "model=%s", model)
		require.Zero(t, body.Data.InputPrice, "model=%s", model)
	}
}
