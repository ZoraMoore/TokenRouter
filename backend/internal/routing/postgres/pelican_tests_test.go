package postgres

import (
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/routing"
)

// TestPelicanTestStoreEmptyScope 覆盖用户没有可见分组时的摘要和按 ID 读取。
func TestPelicanTestStoreEmptyScope(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
		require.NoError(t, mock.ExpectationsWereMet())
	})
	store := NewPelicanTestStore(db)
	for _, ids := range [][]int64{nil, {}} {
		rows, err := store.Latest(t.Context(), ids)
		require.NoError(t, err)
		require.Empty(t, rows)
	}
	detail, err := store.Result(t.Context(), 42, []int64{})
	require.ErrorIs(t, err, routing.ErrPelicanNotFound)
	require.Nil(t, detail)
}
