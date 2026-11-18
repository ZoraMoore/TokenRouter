package apikey

import (
	"context"
	"fmt"
	"time"
)

// RotateCredential 为所属用户原地生成新凭据，保留配置、状态和用量。
// @project-doc docs/domains/identity_and_tenancy.md#api_key_rotation
func (s *APIKeyService) RotateCredential(ctx context.Context, id, userID int64) (*APIKey, error) {
	key, err := s.apiKeyRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
	}
	if key == nil || key.ManagedBy != nil || key.UserID != userID {
		return nil, ErrAPIKeyNotFound
	}
	if key.TeamID != nil {
		key, err = s.KeyHydrateTeamAPIKey(ctx, key, nil)
		if err != nil {
			return nil, err
		}
	}

	oldKey := key.Key
	newKey, err := s.GenerateKey()
	if err != nil {
		return nil, err
	}
	key.Key = newKey
	if err := s.apiKeyRepo.RotateCredential(ctx, key, oldKey); err != nil {
		return nil, fmt.Errorf("rotate api key credential: %w", err)
	}

	// 数据库提交后即使客户端断开，也要完成新旧凭据的缓存失效。
	invalidationCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	s.InvalidateAuthCacheByKey(invalidationCtx, oldKey)
	s.InvalidateAuthCacheByKey(invalidationCtx, newKey)
	return key, nil
}
