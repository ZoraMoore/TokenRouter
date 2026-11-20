package rediscache

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/TokenFlux/TokenRouter/internal/scheduler"
)

const (
	openAIResponsesSessionWindowPrefix = "openai_responses_session_window:"
	sessionMovePrefix                  = "sticky_session_move:"
	sessionMoveReceiptPrefix           = "sticky_session_move_receipt:"
	minimumSessionMoveTTL              = 2 * time.Minute
	sessionMoveReceiptTTL              = 15 * time.Minute
)

// Redis 先核对目标和容量，再比较会话绑定。批次回执与绑定一起提交。
var applyOpenAISessionMovesScript = redis.NewScript(`
local receipt = redis.call('GET', KEYS[4])
if receipt then
  return cjson.decode(receipt)
end

local group_id = tonumber(ARGV[1])
local limit = tonumber(ARGV[3])
if limit < 1 then
  redis.call('SET', KEYS[4], '[0]', 'PX', ARGV[5])
  return {0}
end
local raw = redis.call('GET', KEYS[1])
if not raw then
  return {0}
end
local provider = cjson.decode(raw)
if provider.ID ~= tonumber(ARGV[2]) or provider.Platform ~= 'openai' or provider.Type ~= 'oauth'
    or provider.Status ~= 'active' or provider.Schedulable ~= true or provider.Concurrency ~= limit
    or type(provider.GroupIDs) ~= 'table' then
  return {0}
end
local member = false
for _, id in ipairs(provider.GroupIDs) do
  if id == group_id then
    member = true
    break
  end
end
if not member then
  return {0}
end

local concurrent = redis.call('ZCARD', KEYS[2]) + redis.call('ZCARD', KEYS[3])
local now = redis.call('TIME')
local result = {1, concurrent, tonumber(now[1]), tonumber(now[2])}
local free = limit - concurrent
for index = 5, #KEYS, 3 do
  local binding = KEYS[index]
  local marker = KEYS[index + 1]
  local window = KEYS[index + 2]
  local source = ARGV[6 + (index - 5) / 3]
  local moved = 0
  if free > 0
      and redis.call('GET', binding) == source
      and redis.call('PTTL', binding) > tonumber(ARGV[4])
      and redis.call('EXISTS', marker, window) == 0 then
    redis.call('SET', binding, ARGV[2], 'XX', 'KEEPTTL')
    redis.call('SET', marker, '1', 'PX', ARGV[5])
    moved = 1
    free = free - 1
  end
  result[#result + 1] = moved
end
redis.call('SET', KEYS[4], cjson.encode(result), 'PX', ARGV[5])
return result
`)

// OpenAIResponsesSessionWindowKey 返回普通会话对应的 Responses 上游窗口归属键。
func OpenAIResponsesSessionWindowKey(groupID int64, sessionHash string) string {
	return fmt.Sprintf("%s%d:%s", openAIResponsesSessionWindowPrefix, groupID, sessionHash)
}

// EligibleOpenAISessionMoves 筛选绑定、剩余有效期和上游窗口仍允许迁移的普通会话。
func (c *StickyCache) EligibleOpenAISessionMoves(ctx context.Context, groupID int64, moves []scheduler.OpenAISessionMove, limit int) ([]scheduler.OpenAISessionMove, error) {
	if groupID <= 0 || limit < 1 || len(moves) == 0 {
		return nil, nil
	}

	type bindingRead struct {
		move      scheduler.OpenAISessionMove
		provider  *redis.StringCmd
		ttl       *redis.DurationCmd
		protected *redis.IntCmd
	}
	pipe := c.rdb.Pipeline()
	reads := make([]bindingRead, 0, len(moves))
	for _, move := range moves {
		if !isOpenAISessionMove(move) {
			continue
		}
		binding := openAISessionBindingKey(groupID, move.Hash)
		marker := guardedOpenAISessionMoveKey(groupID, move.Hash)
		window := wsSessionWindowKey(groupID, move)
		reads = append(reads, bindingRead{move, pipe.Get(ctx, binding), pipe.PTTL(ctx, binding), pipe.Exists(ctx, marker, window)})
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}

	eligible := make([]scheduler.OpenAISessionMove, 0, min(limit, len(reads)))
	for _, read := range reads {
		if err := errors.Join(read.ttl.Err(), read.protected.Err()); err != nil {
			return nil, err
		}
		providerID, err := read.provider.Int64()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if providerID == read.move.SourceID && read.ttl.Val() > minimumSessionMoveTTL && read.protected.Val() == 0 {
			eligible = append(eligible, read.move)
			if len(eligible) == limit {
				break
			}
		}
	}
	return eligible, nil
}

// ApplyOpenAISessionMoves 写入当前仍匹配的普通会话绑定并沿用其有效期。
// 并发上限为零时读取批次回执。没有回执时关闭该批，以挡住延迟到达的写入。
func (c *StickyCache) ApplyOpenAISessionMoves(ctx context.Context, batch scheduler.OpenAISessionMoveBatch) (scheduler.OpenAISessionMoveResult, error) {
	if batch.JobID == "" || batch.Number < 1 || batch.GroupID < 1 || batch.ProviderID < 1 || len(batch.Moves) < 1 || len(batch.Moves) > 4 {
		return scheduler.OpenAISessionMoveResult{}, errors.New("invalid OpenAI session move batch")
	}
	keys := []string{
		schedulerProviderKey(strconv.FormatInt(batch.ProviderID, 10)),
		providerSlotKey(batch.ProviderID),
		liveProviderSlotKey(batch.ProviderID),
		fmt.Sprintf("%s%s:%d", sessionMoveReceiptPrefix, batch.JobID, batch.Number),
	}
	args := []any{batch.GroupID, batch.ProviderID, batch.Limit, minimumSessionMoveTTL.Milliseconds(), sessionMoveReceiptTTL.Milliseconds()}
	seen := make(map[string]struct{}, len(batch.Moves))
	for _, move := range batch.Moves {
		_, duplicate := seen[move.Hash]
		if !isOpenAISessionMove(move) || move.SourceID == batch.ProviderID || duplicate {
			return scheduler.OpenAISessionMoveResult{}, errors.New("invalid OpenAI session move candidate")
		}
		seen[move.Hash] = struct{}{}
		keys = append(keys, openAISessionBindingKey(batch.GroupID, move.Hash),
			guardedOpenAISessionMoveKey(batch.GroupID, move.Hash), wsSessionWindowKey(batch.GroupID, move))
		args = append(args, move.SourceID)
	}

	values, err := applyOpenAISessionMovesScript.Run(ctx, c.rdb, keys, args...).Int64Slice()
	if err != nil {
		return scheduler.OpenAISessionMoveResult{}, err
	}
	if len(values) == 1 && values[0] == 0 {
		return scheduler.OpenAISessionMoveResult{}, nil
	}
	if len(values) != len(batch.Moves)+4 || values[0] != 1 {
		return scheduler.OpenAISessionMoveResult{}, errors.New("invalid OpenAI session move receipt")
	}
	result := scheduler.OpenAISessionMoveResult{
		Available: true, Concurrency: int(values[1]), AppliedAt: time.Unix(values[2], values[3]*1000).UTC(),
		Moved: make([]bool, len(batch.Moves)),
	}
	for index := range result.Moved {
		result.Moved[index] = values[index+4] == 1
	}
	return result, nil
}

func isOpenAISessionMove(move scheduler.OpenAISessionMove) bool {
	if len(move.Hash) != 16 || move.SourceID <= 0 || move.APIKeyID <= 0 || strings.ToLower(move.Hash) != move.Hash {
		return false
	}
	_, err := hex.DecodeString(move.Hash)
	return err == nil
}

func openAISessionBindingKey(groupID int64, hash string) string {
	return buildSessionKey(groupID, scheduler.OpenAIStickySessionPrefix+hash)
}

func guardedOpenAISessionMoveKey(groupID int64, hash string) string {
	return fmt.Sprintf("%s%d:%s", sessionMovePrefix, groupID, hash)
}

func wsSessionWindowKey(groupID int64, move scheduler.OpenAISessionMove) string {
	return OpenAIResponsesSessionWindowKey(groupID, scheduler.OpenAIWSSessionWindowHash(move.APIKeyID, move.Hash))
}
