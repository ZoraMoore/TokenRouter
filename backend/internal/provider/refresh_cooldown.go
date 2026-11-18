package provider

import (
	"time"
)

// RefreshCooldownVersion 将成功刷新与交换时的临时停调状态绑定，清理前复核版本。
type RefreshCooldownVersion struct {
	CredentialVersion
	ParentProviderID *int64
	QuotaDimension   string
	Until            *time.Time
	Reason           string
}

func ObserveRefreshCooldown(value *Record) RefreshCooldownVersion {
	if value == nil {
		return RefreshCooldownVersion{}
	}
	return RefreshCooldownVersion{
		CredentialVersion: FailureVersion(value).CredentialVersion,
		ParentProviderID:  clonePointer(value.ParentProviderID), QuotaDimension: value.QuotaDimension,
		Until: clonePointer(value.TempUnschedulableUntil), Reason: value.TempUnschedulableReason,
	}
}
