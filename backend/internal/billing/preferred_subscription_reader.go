package billing

import "context"

// PreferredSubscriptionReader 查询指定用户、分组和订阅的匹配结果。
type PreferredSubscriptionReader interface {
	ResolvePreferredSubscriptionForGroup(context.Context, int64, int64, int64) (*UserSubscription, error)
}

// ResolvePreferredSubscription 查询指定订阅，输入无效、读取失败或订阅不存在时返回 nil。
func ResolvePreferredSubscription(ctx context.Context, reader PreferredSubscriptionReader, userID, subscriptionID int64, groupID *int64) *UserSubscription {
	if reader == nil || userID <= 0 || subscriptionID <= 0 || groupID == nil || *groupID <= 0 {
		return nil
	}
	value, err := reader.ResolvePreferredSubscriptionForGroup(ctx, userID, subscriptionID, *groupID)
	if err != nil {
		return nil
	}
	return value
}
