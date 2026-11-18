package postgres

import (
	"context"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// UpdateUsageSessionWindowEndIfUnchanged 比较身份和窗口后独立提交更新，再尝试发布事件。
func (r *ProviderStore) UpdateUsageSessionWindowEndIfUnchanged(ctx context.Context, v provider.UsageObservationVersion, observed *time.Time, end time.Time) (bool, error) {
	where, values, err := usageObservationPredicate(v, 5, false)
	if err != nil {
		return false, err
	}
	args := []any{v.ID, end, observed, r.options.Now()}
	args = append(args, values...)
	result, err := r.sql.ExecContext(ctx, "UPDATE providers SET session_window_end=$2,updated_at=$4 WHERE id=$1 AND deleted_at IS NULL AND session_window_end IS NOT DISTINCT FROM $3 AND "+where, args...)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil || rows == 0 {
		return false, err
	}
	if err := r.enqueue(ctx, r.sql, ProviderChanged, &v.ID, nil, nil); err != nil {
		r.observe("[SchedulerOutbox] enqueue session window end update failed: provider=%d err=%v", v.ID, err)
	}
	return true, nil
}
