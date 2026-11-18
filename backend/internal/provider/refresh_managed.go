package provider

import (
	"context"
	"errors"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

// WithManagedRefresh 使用共享刷新锁执行管理员流程，锁内重读记录，各写入步骤分别提交。
func (api *OAuthRefreshAPI) WithManagedRefresh(ctx context.Context, observed *Record, key string, apply func(context.Context, *Record) (*Record, string, error)) (*Record, string, error) {
	if err := ValidateManagedRefreshTarget(observed); err != nil {
		return nil, "", err
	}
	if api == nil || api.providerRepo == nil {
		return nil, "", errors.New("oauth refresh provider repository is not configured")
	}
	if observed == nil {
		return nil, "", ErrProviderNotFound
	}
	ctx, finish, err := api.beginRefresh(ctx)
	if err != nil {
		return nil, "", err
	}
	defer finish()
	release, held, err := api.acquireRefreshLock(ctx, observed.ID, key)
	if err != nil {
		return nil, "", err
	}
	if !held {
		defer release()
	}
	current, err := api.providerRepo.GetByID(ctx, observed.ID)
	if err != nil {
		return nil, "", err
	}
	if current == nil || current.ID != observed.ID || current.Platform != observed.Platform || current.Type != observed.Type {
		return nil, "", ErrRefreshProviderStateChanged
	}
	if err := ValidateManagedRefreshTarget(current); err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if held {
		return CloneRecord(current), "", nil
	}
	return apply(ctx, CloneRecord(current))
}

// ValidateManagedRefreshTarget 保持管理刷新在交换前拒绝非 OAuth 和凭据影子的原错误。
func ValidateManagedRefreshTarget(value *Record) error {
	if value == nil {
		return ErrProviderNotFound
	}
	if !value.IsOAuth() && !value.IsQoderCosy() {
		return apperror.BadRequest("NOT_OAUTH", "cannot refresh non-OAuth provider")
	}
	if value.IsCredentialShadow() {
		return apperror.BadRequest("SPARK_SHADOW_NO_REFRESH", "cannot refresh spark shadow provider; its credentials are managed by the parent provider")
	}
	return nil
}
