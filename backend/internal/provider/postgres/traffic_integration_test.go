//go:build integration

package postgres

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/testutil/postgrescontainer"
)

func TestTrafficStoreSelectsCompletedHTTPResponsesWithinOneOwner(t *testing.T) {
	db := postgrescontainer.New(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Second)

	_, err := db.ExecContext(ctx, `
INSERT INTO groups(id,name) VALUES (92,'traffic-one'),(93,'traffic-two');
INSERT INTO users(id,email,password_hash) VALUES (91,'traffic@example.test','test'),(95,'other@example.test','test');
INSERT INTO api_keys(id,user_id,key,name) VALUES (91,91,'traffic-key','test'),(95,95,'other-key','test');
INSERT INTO providers(id,name,platform,type,credentials) VALUES
 (91,'traffic-target','openai','oauth','{}'),(92,'traffic-source','openai','oauth','{}'),(93,'traffic-api','openai','apikey','{}');
INSERT INTO provider_groups(provider_id,group_id) VALUES (91,92),(92,92),(93,92),(92,93);`)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
WITH samples AS (
 SELECT * FROM (VALUES
  ('http-a','move',91,91,92,92,'/v1/responses',FALSE,FALSE,'completed',200,-20),
  ('http-b','move',91,91,92,92,'/v1/responses',FALSE,FALSE,'completed',200,-10),
  ('ws-a','mixed-ws',91,91,92,92,'/v1/responses',FALSE,FALSE,'completed',200,-20),
  ('ws-b','mixed-ws',91,91,92,92,'/v1/responses',FALSE,FALSE,'completed',200,-10),
  ('ws-c','mixed-ws',91,91,92,92,'/v1/responses',TRUE,FALSE,'completed',200,-5),
  ('compact-a','mixed-compact',91,91,92,92,'/v1/responses',FALSE,FALSE,'completed',200,-20),
  ('compact-b','mixed-compact',91,91,92,92,'/v1/responses',FALSE,FALSE,'completed',200,-10),
  ('compact-c','mixed-compact',91,91,92,92,'/v1/responses',FALSE,TRUE,'completed',200,-5),
  ('protocol-a','mixed-protocol',91,91,92,92,'/v1/responses',FALSE,FALSE,'completed',200,-20),
  ('protocol-b','mixed-protocol',91,91,92,92,'/v1/responses',FALSE,FALSE,'completed',200,-10),
  ('protocol-c','mixed-protocol',91,91,92,92,'/v1/chat/completions',FALSE,FALSE,'completed',200,-5),
  ('user-a','two-users',91,91,92,92,'/v1/responses',FALSE,FALSE,'completed',200,-20),
  ('user-b','two-users',91,91,92,92,'/v1/responses',FALSE,FALSE,'completed',200,-10),
  ('user-c','two-users',95,95,92,92,'/v1/responses',FALSE,FALSE,'canceled',200,-5),
  ('source-a','mixed-source',91,91,92,92,'/v1/responses',FALSE,FALSE,'completed',200,-20),
  ('source-b','mixed-source',91,91,92,92,'/v1/responses',FALSE,FALSE,'completed',200,-10),
  ('source-c','mixed-source',91,91,92,93,'/v1/responses',FALSE,FALSE,'completed',200,-5),
  ('other-a','other-group',91,91,93,92,'/v1/responses',FALSE,FALSE,'completed',200,-20),
  ('other-b','other-group',91,91,93,92,'/v1/responses',FALSE,FALSE,'completed',200,-10),
  ('done','move',91,91,92,91,'/v1/responses',FALSE,FALSE,'completed',200,10),
  ('canceled','move',91,91,92,91,'/v1/responses',FALSE,FALSE,'canceled',200,15)
 ) val(req,session_id,user_id,key_id,group_id,provider,endpoint,ws,compact,state,status,at_seconds)
), records AS (
 INSERT INTO request_records(request_id,user_id,started_at,updated_at,revision,record)
 SELECT req,user_id,$1::timestamptz+at_seconds*interval '1 second',$1,1,
        jsonb_build_object('state',state,'status_code',status,'provider_id',provider,'finished_at',$1)
 FROM samples
)
INSERT INTO usage_logs(request_id,session_id,user_id,billing_user_id,api_key_id,group_id,provider_id,inbound_endpoint,openai_ws_mode,native_compaction_v2,
                       model,requested_model,upstream_model,created_at)
SELECT req,session_id,user_id,user_id,key_id,group_id,provider,endpoint,ws,compact,
       'gpt-5','gpt-5','gpt-5',$1::timestamptz+at_seconds*interval '1 second'
 FROM samples`, now)
	require.NoError(t, err)

	store := NewTrafficStore(db)
	candidates, err := store.Candidates(ctx, 91, 92, now.Add(-time.Second))
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "move", candidates[0].ID)
	require.EqualValues(t, 92, candidates[0].SourceID)
	require.EqualValues(t, 91, candidates[0].UserID)
	require.EqualValues(t, 91, candidates[0].APIKeyID)

	_, err = db.ExecContext(ctx, `
INSERT INTO ops_error_logs(group_id,provider_id,error_phase,error_type,upstream_errors,created_at)
VALUES (92,92,'upstream','rate_limit','[{"provider_id":91,"upstream_status_code":429}]'::jsonb,$1)`, now.Add(time.Second))
	require.NoError(t, err)
	requests, status, err := store.Observe(ctx, 91, 92, now)
	require.NoError(t, err)
	require.Equal(t, 429, status)
	require.Len(t, requests, 1)
	require.Equal(t, "move", requests[0].SessionID)
	require.Equal(t, now.Add(10*time.Second), requests[0].StartedAt.UTC())
}
