package postgres

import (
	"context"
	"database/sql"
)

// Executor 接收调用方已经选择的连接或事务，不创建或结束事务。
type Executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}
