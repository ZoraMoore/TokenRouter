package billing

import (
	"log/slog"
	"strings"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/billing/pricing"
)

// applyCatalogTimePricing 在默认目录价上应用数据声明的时段，不影响显式价卡。
func (s *Calculator) applyCatalogTimePricing(price *ModelPricing, at time.Time) *ModelPricing {
	if price == nil || price.TimePricing == nil {
		return price
	}
	location, _ := s.options.LoadLocation(price.TimePricing.Timezone)
	return pricing.MultiplyModelPricing(price, price.TimePricing.MultiplierAt(at, location))
}

func (s *Calculator) operationPrices() pricing.OperationPrices {
	if source, ok := s.catalog.(interface {
		BillingDefaults() pricing.OperationPrices
	}); ok {
		return source.BillingDefaults()
	}
	return pricing.OperationPrices{}
}

// operationPrice 保留显式零价优先；缺价沿用零成本记录并报告原因。
func operationPrice(operation string, configured, catalog *float64) *float64 {
	if configured != nil {
		return configured
	}
	if catalog == nil {
		slog.Warn("operation pricing unavailable", "operation", operation)
	}
	return catalog
}

// audioPrices 按当前操作合并管理员价格与同一目录版本的默认值。
func (s *Calculator) audioPrices(mode string, configured *audioPriceConfig) *audioPriceConfig {
	var result audioPriceConfig
	if configured != nil {
		result = *configured
	}
	defaults := s.operationPrices()
	switch strings.ToLower(mode) {
	case "realtime":
		result.RealtimePerMin = operationPrice(mode, result.RealtimePerMin, defaults.AudioRealtimePricePerMin)
	case "tts":
		result.TTSPerMChars = operationPrice(mode, result.TTSPerMChars, defaults.AudioTTSPricePerMillionChars)
	case "stt":
		result.STTPerHour = operationPrice(mode, result.STTPerHour, defaults.AudioSTTPricePerHour)
	}
	return &result
}
