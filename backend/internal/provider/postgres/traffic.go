package postgres

import (
	"context"
	"database/sql"
	"time"

	"github.com/lib/pq"

	"github.com/TokenFlux/TokenRouter/internal/provider/operation/traffic"
)

const (
	trafficCandidates = `
WITH recent AS (
  SELECT u.id, u.session_id, u.provider_id, u.user_id, u.api_key_id,
         u.model, u.requested_model, u.upstream_model, u.created_at, u.duration_ms,
         COALESCE(p.platform = 'openai' AND p.type = 'oauth' AND p.deleted_at IS NULL, FALSE) AS source_oauth,
         COALESCE(u.inbound_endpoint = '/v1/responses'
           AND u.openai_ws_mode IS FALSE AND u.native_compaction_v2 IS FALSE, FALSE) AS plain_http,
         r.record->>'state' = 'completed'
           AND r.record ? 'finished_at'
           AND r.record->'status_code' BETWEEN '200'::jsonb AND '299'::jsonb
           AND r.record->'provider_id' = to_jsonb(u.provider_id) AS completed
    FROM usage_logs u
    LEFT JOIN providers p ON p.id = u.provider_id
    LEFT JOIN request_records r ON r.request_id = u.request_id
   WHERE u.group_id = $1 AND u.created_at BETWEEN $3::timestamptz - interval '10 minutes' AND $3::timestamptz
     AND u.session_id IS NOT NULL AND u.session_id <> ''
), candidates AS (
  SELECT session_id, (array_agg(provider_id ORDER BY created_at DESC, id DESC))[1] AS source_id,
         min(user_id) AS user_id, min(api_key_id) AS api_key_id,
         array_remove(array_remove(array_agg(DISTINCT model) ||
           array_agg(DISTINCT requested_model) || array_agg(DISTINCT upstream_model), NULL), '') AS models,
         sum(duration_ms) FILTER (WHERE completed) AS busy_time,
         max(created_at) FILTER (WHERE completed) AS last_completed
    FROM recent
   GROUP BY session_id
  HAVING bool_and(source_oauth) AND bool_and(plain_http)
     AND min(user_id) = max(user_id) AND min(api_key_id) = max(api_key_id)
     AND count(*) FILTER (WHERE completed) >= 2
), account_kinds AS (
  SELECT p.id,
         CASE
           WHEN lower(btrim(p.credentials->>'plan_type')) IN ('edu_plus', 'edu', 'education') THEN 'edu'
           WHEN lower(COALESCE(
             NULLIF(btrim(p.credentials->>'email'), ''),
             NULLIF(btrim(p.extra->>'email_address'), ''),
             NULLIF(btrim(p.extra->>'email'), '')
           )) ~ '^[^@[:space:]]+@(gmail|googlemail)[.]com[.]?$' THEN 'gmail'
         END AS kind
    FROM providers p
    JOIN provider_groups pg ON pg.provider_id = p.id AND pg.group_id = $1
   WHERE p.platform = 'openai' AND p.type = 'oauth' AND p.deleted_at IS NULL
)
SELECT c.session_id, c.source_id, c.user_id, c.api_key_id, c.models
  FROM candidates c
  JOIN account_kinds source ON source.id = c.source_id
  JOIN account_kinds target ON target.id = $2 AND target.kind <> source.kind
 WHERE c.last_completed >= $3::timestamptz - interval '90 seconds'
 ORDER BY c.busy_time DESC NULLS LAST, c.last_completed DESC, c.session_id
 LIMIT 256`

	trafficCompleted = `
SELECT DISTINCT u.request_id, u.session_id, u.user_id, u.api_key_id, r.started_at
  FROM usage_logs u
  JOIN request_records r ON r.request_id = u.request_id
 WHERE u.provider_id = $1 AND u.group_id = $2 AND u.created_at >= $3
   AND r.started_at >= $3 AND u.session_id IS NOT NULL AND u.session_id <> ''
   AND u.inbound_endpoint = '/v1/responses'
   AND u.openai_ws_mode IS FALSE AND u.native_compaction_v2 IS FALSE
   AND r.record->>'state' = 'completed' AND r.record ? 'finished_at'
   AND r.record->'status_code' BETWEEN '200'::jsonb AND '299'::jsonb
   AND r.record->'provider_id' = to_jsonb(u.provider_id)`

	trafficRejection = `
SELECT COALESCE(min(status), 0)
  FROM (
    SELECT upstream_status_code AS status
      FROM ops_error_logs
     WHERE provider_id = $1 AND created_at >= $3 AND upstream_status_code IN (401, 429)
    UNION ALL
    SELECT CASE
             WHEN upstream_errors @> jsonb_build_array(jsonb_build_object('provider_id', $1::bigint, 'upstream_status_code', 401)) THEN 401
             WHEN upstream_errors @> jsonb_build_array(jsonb_build_object('provider_id', $1::bigint, 'upstream_status_code', 429)) THEN 429
           END
      FROM ops_error_logs
     WHERE group_id = $2 AND created_at >= $3 AND upstream_errors IS NOT NULL
  ) failures`
)

// TrafficStore 通过用量和请求记录识别同组的已完成 HTTP Responses。
type TrafficStore struct{ db *sql.DB }

// NewTrafficStore 为提供商推流任务建立只读查询。
func NewTrafficStore(db *sql.DB) *TrafficStore { return &TrafficStore{db: db} }

// Candidates 按会话汇总近期的请求，从目标账号所属分组挑选另一类账号的会话。
func (s *TrafficStore) Candidates(ctx context.Context, providerID, groupID int64, now time.Time) ([]traffic.Session, error) {
	rows, err := s.db.QueryContext(ctx, trafficCandidates, groupID, providerID, now)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var sessions []traffic.Session
	for rows.Next() {
		var candidate traffic.Session
		if err := rows.Scan(&candidate.ID, &candidate.SourceID, &candidate.UserID, &candidate.APIKeyID, pq.Array(&candidate.Models)); err != nil {
			return nil, err
		}
		sessions = append(sessions, candidate)
	}
	return sessions, rows.Err()
}

// Observe 返回目标账号完成的 HTTP 请求及观察期内出现的上游拒绝。
func (s *TrafficStore) Observe(ctx context.Context, providerID, groupID int64, since time.Time) ([]traffic.CompletedRequest, int, error) {
	var status int
	if err := s.db.QueryRowContext(ctx, trafficRejection, providerID, groupID, since).Scan(&status); err != nil {
		return nil, 0, err
	}
	rows, err := s.db.QueryContext(ctx, trafficCompleted, providerID, groupID, since)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = rows.Close() }()

	var requests []traffic.CompletedRequest
	for rows.Next() {
		var id string
		var request traffic.CompletedRequest
		if err := rows.Scan(&id, &request.SessionID, &request.UserID, &request.APIKeyID, &request.StartedAt); err != nil {
			return nil, 0, err
		}
		requests = append(requests, request)
	}
	return requests, status, rows.Err()
}
