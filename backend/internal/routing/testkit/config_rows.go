package testkit

import (
	"context"
	"time"

	pricingprovider "github.com/TokenFlux/TokenRouter/internal/billing/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// ConfigRows 为模型配置服务提供可替换的测试数据，服务负责查询缓存。
type ConfigRows struct {
	routing.PricingConfigRepository
	Values    []Configuration
	Platforms map[int64]string
}

func (r ConfigRows) ListAll(context.Context) ([]Configuration, error) { return r.Values, nil }
func (r ConfigRows) GetGroupPlatforms(context.Context, []int64) (map[int64]string, error) {
	return r.Platforms, nil
}

func (r ConfigRows) GetByID(_ context.Context, id int64) (*Configuration, error) {
	for i := range r.Values {
		if r.Values[i].ID == id {
			return r.Values[i].Clone(), nil
		}
	}
	return nil, routing.ErrPricingConfigNotFound
}

// PricingConfig 复制配置并绑定指定分组，注入时钟和定价时区加载器。
func PricingConfig(groupID int64, platform string, value Configuration) *routing.PricingConfigService {
	cloned := value.Clone()
	cloned.GroupIDs = []int64{groupID}
	return NewPricingConfigService(ConfigRows{Values: []Configuration{*cloned}, Platforms: map[int64]string{groupID: platform}}, nil, routing.PricingConfigOptions{Now: time.Now, LoadLocation: pricingprovider.LoadPricingLocation})
}
