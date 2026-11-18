package postgres

import (
	"context"
	"database/sql"
	"time"
)

// getAPIKeyDashboardStatsFromAnalytics 按 Key 当前留存范围组合聚合桶与原始记录。
// @project-doc docs/operations/pre_aggregation.md#query_routing_and_fallback
func (r *Store) getAPIKeyDashboardStatsFromAnalytics(ctx context.Context, keyID int64) (*UserDashboardStats, bool, error) {
	if r.preAggregation == nil || !r.preAggregation.UsageEnabled(ctx) {
		return nil, false, nil
	}
	var oldest sql.NullTime
	if err := scanSingleRow(ctx, r.sql, `SELECT MIN(created_at) FROM usage_logs WHERE api_key_id = $1`, []any{keyID}, &oldest); err != nil {
		return nil, false, err
	}
	stats := &UserDashboardStats{}
	if !oldest.Valid {
		return stats, true, nil
	}
	now := r.calendar.Now()
	total, ok, err := r.openEndedAPIKeyAnalytics(ctx, keyID, oldest.Time, now)
	if err != nil || !ok {
		return nil, false, err
	}
	if err := r.scanAPIKeyAnalytics(ctx, total,
		&stats.TotalRequests, &stats.TotalInputTokens, &stats.TotalOutputTokens,
		&stats.TotalCacheCreationTokens, &stats.TotalCacheReadTokens,
		&stats.TotalCost, &stats.TotalActualCost, &stats.AverageDurationMs); err != nil {
		return nil, false, err
	}
	today := r.calendar.Today()
	if oldest.Time.After(today) {
		today = oldest.Time
	}
	daily, ok, err := r.openEndedAPIKeyAnalytics(ctx, keyID, today, now)
	if err != nil {
		return nil, false, err
	}
	var ignoredAverage float64
	if ok {
		err = r.scanAPIKeyAnalytics(ctx, daily,
			&stats.TodayRequests, &stats.TodayInputTokens, &stats.TodayOutputTokens,
			&stats.TodayCacheCreationTokens, &stats.TodayCacheReadTokens,
			&stats.TodayCost, &stats.TodayActualCost, &ignoredAverage)
	} else {
		// 当天尚无完整聚合小时，读取当天原始记录即可。
		err = scanSingleRow(ctx, r.sql, `SELECT COUNT(*), COALESCE(SUM(input_tokens), 0),
			COALESCE(SUM(output_tokens), 0), COALESCE(SUM(cache_creation_tokens), 0),
			COALESCE(SUM(cache_read_tokens), 0), COALESCE(SUM(total_cost), 0), COALESCE(SUM(actual_cost), 0)
			FROM usage_logs WHERE api_key_id = $1 AND created_at >= $2`, []any{keyID, today},
			&stats.TodayRequests, &stats.TodayInputTokens, &stats.TodayOutputTokens,
			&stats.TodayCacheCreationTokens, &stats.TodayCacheReadTokens,
			&stats.TodayCost, &stats.TodayActualCost)
	}
	return stats, err == nil, err
}

// openEndedAPIKeyAnalytics 复用时间桶划分，原始尾段扩展到未来时间记录。
func (r *Store) openEndedAPIKeyAnalytics(ctx context.Context, keyID int64, start, now time.Time) (usageAnalyticsQuery, bool, error) {
	query, ok, err := r.buildUsageAnalyticsQuery(ctx, UsageLogFilters{APIKeyID: keyID}, start, now, true)
	if ok && err == nil {
		// $2 是原始尾段结束时间；桶范围已按本次 now 和聚合水位确定。
		query.args[1] = "infinity"
	}
	return query, ok, err
}

// scanAPIKeyAnalytics 用非空耗时记录数作为平均值分母。
func (r *Store) scanAPIKeyAnalytics(ctx context.Context, query usageAnalyticsQuery, dest ...any) error {
	return scanSingleRow(ctx, r.sql, query.cte+` SELECT
		COALESCE(SUM(total_requests), 0), COALESCE(SUM(input_tokens), 0), COALESCE(SUM(output_tokens), 0),
		COALESCE(SUM(cache_creation_tokens), 0), COALESCE(SUM(cache_read_tokens), 0),
		COALESCE(SUM(total_cost), 0), COALESCE(SUM(actual_cost), 0),
		COALESCE(SUM(total_duration_ms)::double precision / NULLIF(SUM(duration_count), 0), 0)
		FROM combined `+query.where, query.args, dest...)
}
