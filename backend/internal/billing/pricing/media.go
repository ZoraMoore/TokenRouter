package pricing

// CalculateImageCost 对显式单张价格计算费用，保持乘法顺序与负倍率回退。
func CalculateImageCost(unitPrice float64, imageCount int, rateMultiplier float64) *CostBreakdown {
	if imageCount <= 0 {
		return &CostBreakdown{}
	}
	// 计算总费用
	totalCost := unitPrice * float64(imageCount)

	// 应用倍率（保存时强制 > 0；负数按 0 处理避免按 1x 误扣）
	if rateMultiplier < 0 {
		rateMultiplier = 0
	}
	actualCost := totalCost * rateMultiplier

	return &CostBreakdown{
		TotalCost:   totalCost,
		ActualCost:  actualCost,
		BillingMode: string(BillingModeImage),
	}
}

// CalculateVideoCost 对显式每秒价格计算费用，时长规则使用同一纯定义。
func CalculateVideoCost(perSecondPrice float64, videoCount, durationSeconds int, rateMultiplier float64) *CostBreakdown {
	if videoCount <= 0 {
		return &CostBreakdown{}
	}
	durationSeconds = NormalizeVideoBillingDurationSecondsOrDefault(durationSeconds)
	totalCost := perSecondPrice * float64(durationSeconds) * float64(videoCount)

	if rateMultiplier < 0 {
		rateMultiplier = 0
	}
	actualCost := totalCost * rateMultiplier

	return &CostBreakdown{
		TotalCost:   totalCost,
		ActualCost:  actualCost,
		BillingMode: string(BillingModeVideo),
	}
}

// HasImageUnitPrice 识别目录明确提供的按张价格，显式零价也有效。
func HasImageUnitPrice(catalogPrice *CatalogModelPricing) bool {
	return catalogPrice != nil && (catalogPrice.OutputCostPerImage > 0 ||
		catalogPrice.ImagePricePresent && catalogPrice.OutputCostPerImage == 0)
}

// DefaultImagePrice 按尺寸读取明确单价，不按固定倍率猜测更大尺寸。
func DefaultImagePrice(price *CatalogModelPricing, size string) (float64, bool) {
	if price == nil {
		return 0, false
	}
	if value, ok := price.ImagePrices[NormalizeImageBillingTierOrDefault(size)]; ok {
		return value, true
	}
	if len(price.ImagePrices) > 0 {
		return 0, false
	}
	if HasImageUnitPrice(price) {
		return price.OutputCostPerImage, true
	}
	return 0, false
}

// DefaultVideoPrice 只读取相同型号、相同分辨率的每秒单价。
func DefaultVideoPrice(price *CatalogModelPricing, resolution string) (float64, bool) {
	if price == nil {
		return 0, false
	}
	value, ok := price.VideoPrices[NormalizeVideoBillingResolutionOrDefault(resolution)]
	return value, ok
}
