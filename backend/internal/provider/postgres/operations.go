package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/TokenFlux/TokenRouter/internal/provider/operation"
)

const (
	jobFields = "id, provider_id, provider_name, group_id, kind, state, stage, message, checkpoint, result"
)

// OperationStore 持久化提供商操作、处理进度和恢复数据。
type OperationStore struct {
	db *sql.DB
}

type jobScanner interface {
	Scan(...any) error
}

// NewOperationStore 使用应用连接池读写管理员任务。
func NewOperationStore(db *sql.DB) *OperationStore {
	return &OperationStore{db: db}
}

// Enqueue 复用同账号的在途任务，返回记录供调用方核对操作类型和分组。
func (s *OperationStore) Enqueue(ctx context.Context, job operation.Job) (operation.Job, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return operation.Job{}, err
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO provider_operations (id, provider_id, provider_name, group_id, kind, state)
		VALUES ($1, $2, $3, $4, $5, 'queued')
		ON CONFLICT (provider_id) WHERE state IN ('queued', 'running')
		DO UPDATE SET provider_id = EXCLUDED.provider_id
		RETURNING `+jobFields, id, job.ProviderID, job.ProviderName, job.GroupID, job.Kind)
	return scanJob(row)
}

// Active 查询同一账号当前的任务，在外部资料核验前可复用该任务。
func (s *OperationStore) Active(ctx context.Context, providerID int64) (operation.Job, error) {
	return scanJob(s.db.QueryRowContext(ctx, `SELECT `+jobFields+`
		FROM provider_operations WHERE provider_id = $1 AND state IN ('queued', 'running')`, providerID))
}

// Claim 找到下一项已到执行时间的任务。running 也可续跑，调用方须持同类数据库锁。
func (s *OperationStore) Claim(ctx context.Context, kind operation.Kind) (operation.Job, error) {
	return scanJob(s.db.QueryRowContext(ctx, `
		UPDATE provider_operations SET state = 'running', message = ''
		WHERE id = (
			SELECT id FROM provider_operations
			WHERE kind = $1 AND state IN ('queued', 'running') AND next_at <= now()
			ORDER BY next_at, created_at, id LIMIT 1
		)
		RETURNING `+jobFields, kind))
}

// SaveCheckpoint 在外部调用前持久化阶段和续跑数据。
func (s *OperationStore) SaveCheckpoint(ctx context.Context, id, stage string, data json.RawMessage) error {
	updated, err := s.db.ExecContext(ctx, `
		UPDATE provider_operations SET stage = $2, checkpoint = $3::jsonb
		WHERE id = $1 AND state = 'running'`, id, stage, string(data))
	if err != nil {
		return err
	}
	count, err := updated.RowsAffected()
	if err == nil && count != 1 {
		return operation.ErrNotFound
	}
	return err
}

// Finish 写入显示结果和下一次轮询时间。
func (s *OperationStore) Finish(ctx context.Context, id string, state operation.State, message string, result operation.Result, delay time.Duration) error {
	raw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE provider_operations SET state = $2, message = $3, result = $4::jsonb,
			next_at = now() + ($5::bigint * INTERVAL '1 millisecond')
		WHERE id = $1 AND state = 'running'`, id, state, message, string(raw), delay.Milliseconds())
	return err
}

// Get 读取保存的处理进度。
func (s *OperationStore) Get(ctx context.Context, id string) (operation.Job, error) {
	return scanJob(s.db.QueryRowContext(ctx, "SELECT "+jobFields+" FROM provider_operations WHERE id = $1", id))
}

// List 返回最近的任务，按账号筛选时也显示已重登到该账号的记录。
func (s *OperationStore) List(ctx context.Context, providerID int64) ([]operation.Job, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+jobFields+` FROM provider_operations
		WHERE $1::bigint = 0 OR provider_id = $1 OR result->>'provider_id' = ($1::bigint)::text
		ORDER BY created_at DESC, id LIMIT 100`, providerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]operation.Job, 0)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func scanJob(row jobScanner) (operation.Job, error) {
	var job operation.Job
	var result []byte
	err := row.Scan(&job.ID, &job.ProviderID, &job.ProviderName, &job.GroupID, &job.Kind, &job.State,
		&job.Stage, &job.Message, &job.Data, &result)
	if errors.Is(err, sql.ErrNoRows) {
		return operation.Job{}, operation.ErrNotFound
	}
	if err != nil {
		return operation.Job{}, err
	}
	return job, json.Unmarshal(result, &job.Result)
}
