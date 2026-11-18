package provider

import "context"

type CredentialUpdateStore interface {
	Update(context.Context, *Record) error
}
type CredentialFieldsUpdater interface {
	UpdateCredentials(context.Context, int64, map[string]any) error
}

// PersistCredentials 拒绝影子写入，先更新传入记录再保存凭据，返回值报告记录是否已更新。
// 交换结果的身份比较由刷新用例的条件操作承担，普通配置拥有者可使用此专用字段写入。
func PersistCredentials(ctx context.Context, store CredentialUpdateStore, value *Record, credentials map[string]any, warn func(string, ...any)) (bool, error) {
	if store == nil || value == nil {
		return false, nil
	}
	if value.IsCredentialShadow() {
		if warn != nil {
			warn("skip persisting credentials to spark shadow provider", "provider_id", value.ID, "parent_id", *value.ParentProviderID)
		}
		return false, nil
	}
	value.Credentials = CloneValues(credentials)
	if value.Credentials == nil {
		value.Credentials = map[string]any{}
	}
	if updater, ok := store.(CredentialFieldsUpdater); ok {
		return true, updater.UpdateCredentials(ctx, value.ID, value.Credentials)
	}
	return true, store.Update(ctx, value)
}
