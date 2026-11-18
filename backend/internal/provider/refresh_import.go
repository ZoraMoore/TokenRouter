package provider

import (
	"context"
	"errors"
	"fmt"
)

// RefreshImported 对已落库的导入身份进行一次尽力交换。它共享生产刷新锁和停止预算，
// 导入会刷新禁用提供商的凭据，并写入版本字段，刷新资格按导入流程判断。
func (api *OAuthRefreshAPI) RefreshImported(ctx context.Context, value *Record, key string, exchange func(context.Context, *Record) map[string]any) error {
	if value == nil || value.IsCredentialShadow() || exchange == nil {
		return nil
	}
	if api == nil || api.providerRepo == nil {
		return errors.New("oauth refresh provider repository is not configured")
	}
	ctx, finish, err := api.beginRefresh(ctx)
	if err != nil {
		return err
	}
	defer finish()
	release, held, err := api.acquireRefreshLock(ctx, value.ID, key)
	if err != nil {
		return err
	}
	if held {
		return nil
	}
	defer release()
	current, err := api.providerRepo.GetByID(ctx, value.ID)
	if err != nil {
		return err
	}
	if current == nil || current.ID != value.ID || current.Status != value.Status || RefreshCredentialIdentity(current) != RefreshCredentialIdentity(value) {
		return ErrRefreshProviderStateChanged
	}
	attempted := snapshotRefreshRecord(current)
	if err := ctx.Err(); err != nil {
		return err
	}
	credentials := exchange(ctx, current)
	if err := ctx.Err(); err != nil {
		return err
	}
	if credentials == nil {
		return nil
	}
	writer, ok := api.providerRepo.(CredentialRefreshWriter)
	if !ok {
		return fmt.Errorf("%w: conditional credential writer is not configured", ErrRefreshCredentialPersist)
	}
	applied, err := writer.UpdateOAuthCredentialsIfUnchanged(ctx, CredentialVersion{ID: attempted.ID, Platform: attempted.Platform, Type: attempted.Type, Status: attempted.Status, ProxyID: attempted.ProxyID, Credentials: attempted.Credentials}, CloneValues(credentials))
	if err != nil {
		return err
	}
	if !applied {
		// 比较失败时返回当前状态，本次 token 交换结果丢弃。
		latest, readErr := api.providerRepo.GetByID(ctx, value.ID)
		if readErr != nil {
			return readErr
		}
		if latest == nil || latest.ID != value.ID {
			return ErrRefreshProviderStateChanged
		}
		return nil
	}
	return nil
}
