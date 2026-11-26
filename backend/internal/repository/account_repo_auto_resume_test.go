package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestSetErrorMarksSchedulingAutoDisabled(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec(`(?s)UPDATE accounts\s+SET status = \$1.*jsonb_set\(COALESCE\(extra, '\{\}'::jsonb\), '\{scheduling_auto_disabled\}', 'true'::jsonb\)`).
		WithArgs(service.StatusError, "boom", int64(7)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(7), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	require.NoError(t, repo.SetError(context.Background(), 7, "boom"))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResumeSchedulableIfAutoDisabledResumesMarkedAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// 判布尔用文本比较而非 ::boolean：extra 是管理员可编辑的 JSON，非法值不能让整条 UPDATE 报错。
	mock.ExpectExec(`(?s)UPDATE accounts\s+SET schedulable = TRUE.*scheduling_auto_disabled.*extra->>'scheduling_auto_disabled' = 'true'`).
		WithArgs(int64(7), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventAccountChanged, int64(7), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	resumed, err := repo.ResumeSchedulableIfAutoDisabled(context.Background(), 7, time.Now())
	require.NoError(t, err)
	require.True(t, resumed)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResumeSchedulableIfAutoDisabledSkipsManuallyPausedAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	mock.ExpectExec(`(?s)UPDATE accounts\s+SET schedulable = TRUE`).
		WithArgs(int64(7), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 0))

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	resumed, err := repo.ResumeSchedulableIfAutoDisabled(context.Background(), 7, time.Now())
	require.NoError(t, err)
	require.False(t, resumed)
	require.NoError(t, mock.ExpectationsWereMet())
}
