package transfer

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/egress"
)

const (
	DataType = "tokenrouter-data"
	// LegacyDataType 标识 Sub2API 导出的归档。
	LegacyDataType = "sub2api-data"
	DataVersion    = 2
)

type (
	DataProxy       = egress.TransferProxy
	DataImportError = egress.TransferError
)

type DataPayload struct {
	Type       string         `json:"type,omitempty"`
	Version    int            `json:"version,omitempty"`
	ExportedAt string         `json:"exported_at"`
	Proxies    []DataProxy    `json:"proxies"`
	Providers  []DataProvider `json:"providers"`
	// SkippedShadows 记录导出时被排除的 spark 影子提供商数量(见 ExportData)。仅作可见性提示,
	// 导入侧忽略该字段;omitempty 保持向后兼容。
	SkippedShadows int `json:"skipped_shadows,omitempty"`
}

// DataProvider 用于管理员备份导出，Credentials 包含凭据原文。
// 格式不包含 parent_provider_id 或 quota_dimension，导入要求凭据非空，无法重建影子的父子链接。
// Spark 影子及其独立优先级、并发、分组和状态不在导出范围，前端会提示跳过的影子数量。
type DataProvider struct {
	Name               string         `json:"name"`
	Notes              *string        `json:"notes,omitempty"`
	Platform           string         `json:"platform"`
	Type               string         `json:"type"`
	Credentials        map[string]any `json:"credentials"`
	Extra              map[string]any `json:"extra,omitempty"`
	ProxyKey           *string        `json:"proxy_key,omitempty"`
	Concurrency        *int           `json:"concurrency,omitempty"`
	Priority           *int           `json:"priority,omitempty"`
	RateMultiplier     *float64       `json:"rate_multiplier,omitempty"`
	ExpiresAt          *int64         `json:"expires_at,omitempty"`
	AutoPauseOnExpired *bool          `json:"auto_pause_on_expired,omitempty"`
}

type DataImportRequest struct {
	Data DataPayload `json:"data"`
}
type DataImportResult struct {
	ProxyCreated    int               `json:"proxy_created"`
	ProxyReused     int               `json:"proxy_reused"`
	ProxyFailed     int               `json:"proxy_failed"`
	ProviderCreated int               `json:"provider_created"`
	ProviderFailed  int               `json:"provider_failed"`
	Errors          []DataImportError `json:"errors,omitempty"`
}

// UnmarshalJSON 读取导入条目，并把旧 OpenAI 套餐字段补入凭据。
func (a *DataProvider) UnmarshalJSON(data []byte) error {
	type dataProviderAlias DataProvider

	var decoded struct {
		dataProviderAlias
		PlanType string `json:"plan_type"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}

	*a = DataProvider(decoded.dataProviderAlias)
	if a.Platform == "openai" && a.Type == "oauth" && len(a.Credentials) > 0 {
		current, _ := a.Credentials["plan_type"].(string)
		if strings.TrimSpace(current) == "" && strings.TrimSpace(decoded.PlanType) != "" {
			a.Credentials["plan_type"] = strings.TrimSpace(decoded.PlanType)
		}
	}
	return nil
}

// ValidateHeader 校验导入的类型、版本和字段，接受兼容的品牌名称。
// @project-doc docs/interfaces/http_api.md#product_name_compatibility
func ValidateHeader(payload DataPayload) error {
	if payload.Type != DataType && payload.Type != LegacyDataType {
		return fmt.Errorf("unsupported data type: %s", payload.Type)
	}
	if payload.Version != DataVersion {
		return fmt.Errorf("unsupported data version: %d", payload.Version)
	}
	if payload.Proxies == nil {
		return errors.New("proxies is required")
	}
	if payload.Providers == nil {
		return errors.New("providers is required")
	}
	return nil
}

// UnmarshalJSON 把旧 Sub2API 账号归档转换为版本 2 的提供商集合。
// 两种集合同时出现时拒绝导入，以免遗漏其中一组数据。
func (p *DataPayload) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if _, legacy := fields["accounts"]; legacy {
		if _, mixed := fields["providers"]; mixed {
			return errors.New("accounts and providers cannot appear together")
		}
		var value struct {
			Type           string         `json:"type"`
			Version        *int           `json:"version"`
			ExportedAt     string         `json:"exported_at"`
			Proxies        []DataProxy    `json:"proxies"`
			Accounts       []DataProvider `json:"accounts"`
			SkippedShadows int            `json:"skipped_shadows"`
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return err
		}
		if value.Type != "" && value.Type != LegacyDataType {
			return errors.New("unsupported legacy data type")
		}
		if value.Version != nil && *value.Version != 1 {
			return errors.New("unsupported legacy data version")
		}
		if value.Accounts == nil {
			return errors.New("accounts must be an array")
		}
		if value.Proxies == nil {
			value.Proxies = []DataProxy{}
		}
		*p = DataPayload{Type: DataType, Version: DataVersion, ExportedAt: value.ExportedAt, Proxies: value.Proxies, Providers: value.Accounts, SkippedShadows: value.SkippedShadows}
		return nil
	}
	type payload DataPayload
	var value payload
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*p = DataPayload(value)
	return nil
}
