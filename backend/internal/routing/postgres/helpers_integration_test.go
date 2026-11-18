//go:build integration

package postgres

import (
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"
)

// newPelicanGroup 在子测试结束时连同计划和结果一起清理分组。
func newPelicanGroup(t *testing.T, db *sql.DB, name string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, db.QueryRowContext(t.Context(), "INSERT INTO groups(name) VALUES($1) RETURNING id", name).Scan(&id))
	t.Cleanup(func() {
		_, err := db.Exec("DELETE FROM groups WHERE id=$1", id)
		require.NoError(t, err)
	})
	return id
}
