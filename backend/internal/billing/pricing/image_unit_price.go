package pricing

import (
	"fmt"
	"math"
)

// ConfiguredImageUnitPrice 返回单张价格及是否找到价格，零价仍视为已定价。
func ConfiguredImageUnitPrice(resolved *ResolvedPricing, size string) (float64, bool) {
	if resolved == nil || resolved.Mode != BillingModeImage && resolved.Mode != BillingModePerRequest {
		return 0, false
	}
	return ResolveRequestUnitPrice(resolved, size, nil)
}

// ValidateImageUnitPrice 校验固定单张价格。
func ValidateImageUnitPrice(price float64) (float64, error) {
	if math.IsNaN(price) || math.IsInf(price, 0) || price < 0 {
		return 0, fmt.Errorf("invalid image unit price: %w", ErrModelPricingUnavailable)
	}
	return price, nil
}

// ResolveImageUnitPrice 优先读取配置单价，图片模式或无按次价卡时可补充目录价。按次缺价返回错误。
func ResolveImageUnitPrice(resolved *ResolvedPricing, catalog *CatalogModelPricing, size string) (float64, error) {
	price, found := ConfiguredImageUnitPrice(resolved, size)
	if !found && (resolved == nil || resolved.Mode != BillingModePerRequest) {
		price, found = DefaultImagePrice(catalog, size)
	}
	if !found {
		return 0, ErrModelPricingUnavailable
	}
	return ValidateImageUnitPrice(price)
}
