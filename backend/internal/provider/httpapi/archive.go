package httpapi

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	idempotencyhttp "github.com/TokenFlux/TokenRouter/internal/idempotency/httpapi"
	infraerrors "github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/provider/httpapi/dto"
	"github.com/TokenFlux/TokenRouter/internal/provider/transfer"
	response "github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// ArchiveHandler 只负责备份 HTTP 输入、幂等和管理员响应；资源查询与导入由提供商用例拥有。
type ArchiveHandler struct {
	idempotencyhttp.Executor
	archive *provider.Archive
}

func NewArchiveHandler(archive *provider.Archive) *ArchiveHandler {
	return &ArchiveHandler{archive: archive}
}

func (h *ArchiveHandler) ExportData(c *gin.Context) {
	format := c.Query("format")
	if format != "" && format != "tokenrouter" && format != "sub2api" {
		response.BadRequest(c, "unsupported provider export format")
		return
	}
	ids, err := parseProviderIDs(c)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	query := provider.ArchiveExportQuery{IDs: ids, Platform: c.Query("platform"), Type: c.Query("type"), Status: c.Query("status"), PrivacyMode: strings.TrimSpace(c.Query("privacy_mode")), Search: strings.TrimSpace(c.Query("search")), SortBy: c.DefaultQuery("sort_by", "name"), SortOrder: c.DefaultQuery("sort_order", "asc"), IncludeProxies: func() (bool, error) { return parseIncludeProxies(c) }}
	if len(query.Search) > 100 {
		query.Search = query.Search[:100]
	}
	if len(ids) == 0 {
		if group := c.Query("group"); group != "" {
			if group == "ungrouped" {
				query.GroupID = provider.ProviderListGroupUngrouped
			} else {
				value, parseErr := strconv.ParseInt(group, 10, 64)
				if parseErr != nil || value <= 0 {
					response.ErrorFrom(c, infraerrors.BadRequest("INVALID_GROUP_FILTER", "invalid group filter"))
					return
				}
				query.GroupID = value
			}
		}
	}
	payload, err := h.archive.Export(c.Request.Context(), query)
	if err != nil {
		if inputErr, ok := errors.AsType[*provider.ArchiveInputError](err); ok {
			response.BadRequest(c, inputErr.Error())
		} else {
			response.ErrorFrom(c, err)
		}
		return
	}
	if format == "sub2api" {
		response.Success(c, sub2APIArchive(payload))
		return
	}
	response.Success(c, payload)
}

// sub2APIArchive 将管理员选中的提供商写入 Sub2API 的 accounts 集合。
// @project-doc docs/interfaces/http_api.md#product_name_compatibility
func sub2APIArchive(payload transfer.DataPayload) transfer.Sub2APIDataPayload {
	for i := range payload.Providers {
		extra := dto.RedactProviderManagedExtra(payload.Providers[i].Extra)
		// Codex 临时票据和采集代理可能携带其他会话的凭据。
		for key := range extra {
			if strings.HasPrefix(key, "codex_turn_ticket:") || key == "codex_harvest_proxy_url" {
				delete(extra, key)
			}
		}
		// TokenRouter 的账号通过 proxy_key 绑定代理。导入过的 Sub2API 代理组 ID 属于来源数据库。
		delete(extra, "openai_proxy_ids")
		delete(extra, "openai_proxy_mode")
		delete(extra, "openai_excel_bps_proxy_ids")
		delete(extra, "openai_excel_bps_proxy_mode")
		payload.Providers[i].Extra = extra
	}
	return transfer.Sub2APIDataPayload{
		ExportedAt:     payload.ExportedAt,
		Proxies:        payload.Proxies,
		Accounts:       payload.Providers,
		SkippedShadows: payload.SkippedShadows,
	}
}

func (h *ArchiveHandler) ImportData(c *gin.Context) {
	var req transfer.DataImportRequest
	if err := response.BindJSONStrict(c, &req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	if err := transfer.ValidateHeader(req.Data); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	// 废弃提供商字段不参与导入幂等指纹，旧客户端的值或非法类型都与缺省输入等价。
	for index := range req.Data.Providers {
		provider.DiscardDeprecatedExtra(req.Data.Providers[index].Extra)
	}

	h.ExecuteAdminIdempotentJSON(c, "admin.providers.import_data", req, h.DefaultWriteIdempotencyTTL(), func(ctx context.Context) (any, error) {
		return h.archive.Import(ctx, req)
	})
}

func parseProviderIDs(c *gin.Context) ([]int64, error) {
	values := c.QueryArray("ids")
	if len(values) == 0 {
		raw := strings.TrimSpace(c.Query("ids"))
		if raw != "" {
			values = []string{raw}
		}
	}
	if len(values) == 0 {
		return nil, nil
	}

	ids := make([]int64, 0, len(values))
	for _, item := range values {
		for part := range strings.SplitSeq(item, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil || id <= 0 {
				return nil, fmt.Errorf("invalid provider id: %s", part)
			}
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func parseIncludeProxies(c *gin.Context) (bool, error) {
	raw := strings.TrimSpace(strings.ToLower(c.Query("include_proxies")))
	if raw == "" {
		return true, nil
	}
	switch raw {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return true, fmt.Errorf("invalid include_proxies value: %s", raw)
	}
}
