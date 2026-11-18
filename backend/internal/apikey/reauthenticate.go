package apikey

import (
	"context"
)

// Reauthenticate 直接回源复核长连接身份；不修改已放行轮次的归属和计费快照。
func (s *APIKeyService) Reauthenticate(ctx context.Context, previous *APIKey, input AuthenticationInput) (*APIKey, error) {
	if s == nil || previous == nil || previous.User == nil || previous.Key == "" || len(previous.Key) > MaxAPIKeyCredentialBytes {
		return nil, ErrAPIKeyNotFound
	}
	if !s.operations.enter() {
		return nil, ErrAuthenticationStopped
	}
	defer s.operations.leave()

	key, err := s.KeyLookupAPIKeyForAuth(ctx, previous.Key)
	if err != nil {
		return nil, err
	}
	if key == nil || key.ID != previous.ID || key.UserID != previous.UserID || !sameKeyID(key.TeamID, previous.TeamID) {
		return nil, ErrAPIKeyNotFound
	}
	key = CopyAPIKey(key)
	key.Key = previous.Key
	s.KeyCompileAPIKeyIPRules(key)
	if _, err := s.authenticateKey(key, input); err != nil {
		return nil, err
	}
	// 付款主体或结算来源变化必须重连，避免旧连接按旧归属继续消费。
	if key.User.ID != previous.User.ID || APIKeyEffectiveBillingMode(key) != APIKeyEffectiveBillingMode(previous) || !sameKeyID(key.PreferredSubscriptionID, previous.PreferredSubscriptionID) {
		return nil, ErrAPIKeyNotFound
	}
	if err := s.CheckAPIKeyQuotaAndExpiry(key); err != nil {
		return nil, err
	}
	if key.Status == StatusAPIKeyExpired {
		return nil, ErrAPIKeyExpired
	}
	if key.Status == StatusAPIKeyQuotaExhausted {
		return nil, ErrAPIKeyQuotaExhausted
	}
	// 连接保持已选分组；复合 Key 与回退分组仍须通过当前权限检查。
	if previous.GroupID != nil {
		key, err = s.ResolveRuntimeGroup(ctx, key, *previous.GroupID)
		if err != nil {
			return nil, err
		}
	}
	return key, nil
}

func sameKeyID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
