package query

import (
	"context"
	"fmt"
	"time"

	entsql "entgo.io/ent/dialect/sql"
	"github.com/lib/pq"

	dbuser "github.com/TokenFlux/TokenRouter/ent/user"
	infra "github.com/TokenFlux/TokenRouter/internal/infra/postgres"
	"github.com/TokenFlux/TokenRouter/internal/pkg/pagination"
)

// IdentityUserLastUsedAtOrder 按事务内维护的用户活动时间排序，无记录用户按空值排序。
// @project-doc docs/operations/observability_and_data_lifecycle.md#user_activity_summary
func IdentityUserLastUsedAtOrder(sortOrder string) []func(*entsql.Selector) {
	orderExpr := func(direction, nulls string, tieOrder func(string) string) func(*entsql.Selector) {
		return func(s *entsql.Selector) {
			activity := entsql.Table("usage_user_activity").As("user_activity")
			s.LeftJoin(activity).On(s.C(dbuser.FieldID), activity.C("user_id"))
			s.OrderExpr(entsql.Expr(activity.C("last_used_at") + " " + direction + " NULLS " + nulls))
			s.OrderBy(tieOrder(s.C(dbuser.FieldID)))
		}
	}

	if sortOrder == pagination.SortOrderAsc {
		return []func(*entsql.Selector){
			orderExpr("ASC", "FIRST", entsql.Asc),
		}
	}
	return []func(*entsql.Selector){
		orderExpr("DESC", "LAST", entsql.Desc),
	}
}

// GetLatestUsedAtByUserIDs 批量读取用户活动汇总，缺少记录的用户不进入结果。
func GetLatestUsedAtByUserIDs(ctx context.Context, db infra.Executor, userIDs []int64) (map[int64]*time.Time, error) {
	result := make(map[int64]*time.Time, len(userIDs))
	if len(userIDs) == 0 {
		return result, nil
	}
	if db == nil {
		return nil, fmt.Errorf("sql executor is not configured")
	}

	const query = `
		SELECT user_id, last_used_at
		FROM usage_user_activity
		WHERE user_id = ANY($1)
	`

	rows, err := db.QueryContext(ctx, query, pq.Array(userIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			userID     int64
			lastUsedAt time.Time
		)
		if scanErr := rows.Scan(&userID, &lastUsedAt); scanErr != nil {
			return nil, scanErr
		}
		ts := lastUsedAt.UTC()
		result[userID] = &ts
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetRecentUserActivity 从共享活动汇总补齐进程启动前的活跃用户窗口。
func GetRecentUserActivity(ctx context.Context, db infra.Executor, since time.Time) (map[int64]time.Time, error) {
	if db == nil {
		return nil, fmt.Errorf("sql executor is not configured")
	}
	rows, err := db.QueryContext(ctx, `SELECT user_id, last_used_at FROM usage_user_activity WHERE last_used_at >= $1`, since)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make(map[int64]time.Time)
	for rows.Next() {
		var id int64
		var at time.Time
		if err := rows.Scan(&id, &at); err != nil {
			return nil, err
		}
		result[id] = at.UTC()
	}
	return result, rows.Err()
}
