package session

import (
	"context"
	"strings"
)

// HTTPResponseOwnerReader 读取 HTTP 响应所属的用户和 API Key。
type HTTPResponseOwnerReader interface {
	GetHTTPResponseOwner(context.Context, int64, string) (int64, int64, bool, error)
}

// ValidateHTTPResponseOwner 允许同一用户跨 Key 续接，缺少用户归属的历史记录按 Key 检查。
// 输入无效时直接返回 false，其余情况读取一次存储。
func ValidateHTTPResponseOwner(ctx context.Context, read func() HTTPResponseOwnerReader, groupID int64, responseID string, userID, keyID int64) (bool, error) {
	if read == nil || strings.TrimSpace(responseID) == "" || userID <= 0 || keyID <= 0 {
		return false, nil
	}
	store := read()
	if store == nil {
		return false, nil
	}
	ownerUserID, ownerKeyID, found, err := store.GetHTTPResponseOwner(ctx, groupID, responseID)
	if err != nil || !found {
		return false, err
	}
	return ownerUserID == userID || (ownerUserID <= 0 && ownerKeyID == keyID), nil
}
