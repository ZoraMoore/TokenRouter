package batchimage

import (
	"context"
	"fmt"

	"github.com/TokenFlux/TokenRouter/internal/billing"
)

// ImagePrices 根据价卡和模型返回图片单价。
type ImagePrices interface {
	ResolveImageUnitPrice(context.Context, billing.PricingInput, string) (float64, error)
}

// Pricing 保留任务报价的按需分组读取顺序，价格算法仍由 billing 唯一实现。
type Pricing struct {
	Resolver  ImagePrices
	GroupRepo GroupReader
}

func (r *Pricing) BatchImageUnitPrice(ctx context.Context, input BatchImagePriceInput) (float64, error) {
	if r == nil || r.Resolver == nil {
		return 0, ErrBatchImageSettlementPricingMissing
	}
	price, err := r.Resolver.ResolveImageUnitPrice(ctx, billing.PricingInput{Model: input.Model, GroupID: input.GroupID}, input.ImageSize)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrBatchImageSettlementPricingMissing, err)
	}
	return price, nil
}
