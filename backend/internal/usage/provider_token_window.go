package usage

import (
	"context"
	"time"
)

// ProviderTokenWindowReader 保留旧统计入口及可选的批量读取能力。
type ProviderTokenWindowReader interface {
	GetProviderWindowStats(context.Context, int64, time.Time) (*ProviderStats, error)
}

// ReadProviderTokenWindow 优先批量读取；批量失败不额外执行逐提供商查询。
func ReadProviderTokenWindow(ctx context.Context, reader ProviderTokenWindowReader, ids []int64, start time.Time) (map[int64]int64, error) {
	if reader == nil {
		return nil, nil
	}
	var values map[int64]*ProviderStats
	if batch, ok := reader.(interface {
		GetProviderWindowStatsBatch(context.Context, []int64, time.Time) (map[int64]*ProviderStats, error)
	}); ok {
		var err error
		values, err = batch.GetProviderWindowStatsBatch(ctx, ids, start)
		if err != nil {
			return nil, err
		}
	} else {
		values = make(map[int64]*ProviderStats, len(ids))
		for _, id := range ids {
			value, err := reader.GetProviderWindowStats(ctx, id, start)
			if err != nil {
				return nil, err
			}
			values[id] = value
		}
	}
	tokens := make(map[int64]int64, len(values))
	for id, value := range values {
		if value != nil {
			tokens[id] = value.Tokens
		}
	}
	return tokens, nil
}
