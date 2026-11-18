package query

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestGetRecentUserActivityUsesSharedSummaryAndTimeWindow(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer func() { _ = db.Close() }()
	since := time.Now().UTC().Add(-5 * time.Minute)
	at := since.Add(time.Minute)
	mock.ExpectQuery(`SELECT user_id, last_used_at FROM usage_user_activity WHERE last_used_at >= \$1`).WithArgs(since).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "last_used_at"}).AddRow(7, at).AddRow(9, at))
	users, err := GetRecentUserActivity(context.Background(), db, since)
	require.NoError(t, err)
	require.Equal(t, map[int64]time.Time{7: at, 9: at}, users)
	require.NoError(t, mock.ExpectationsWereMet())
}
