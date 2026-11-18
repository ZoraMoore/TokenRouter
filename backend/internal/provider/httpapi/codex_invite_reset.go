package httpapi

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	response "github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// CodexInviteResetCommands 是管理 HTTP 对提供商用例的窄操作接口。
type CodexInviteResetCommands interface {
	GetStatus(context.Context, int64) (*provider.CodexInviteResetStatus, error)
	SendInvite(context.Context, int64, provider.CodexInviteRequest) (*provider.CodexInviteResetInviteResult, error)
	Consume(context.Context, int64, string) (*provider.CodexInviteResetConsumeResult, error)
}

// CodexInviteResetHandler 处理 Codex 邀请重置管理接口。
type CodexInviteResetHandler struct {
	service CodexInviteResetCommands
}

// NewCodexInviteResetHandler 创建 Codex 邀请重置管理处理器。
func NewCodexInviteResetHandler(service CodexInviteResetCommands) *CodexInviteResetHandler {
	return &CodexInviteResetHandler{service: service}
}

// GetStatus 查询当前提供商的邀请资格和可用重置次数。
func (h *CodexInviteResetHandler) GetStatus(c *gin.Context) {
	providerID, ok := parseCodexInviteResetProviderID(c)
	if !ok {
		return
	}
	result, err := h.service.GetStatus(c.Request.Context(), providerID)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}

// SendInvite 发送 Codex 邀请邮件。
func (h *CodexInviteResetHandler) SendInvite(c *gin.Context) {
	providerID, ok := parseCodexInviteResetProviderID(c)
	if !ok {
		return
	}
	var req provider.CodexInviteRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	if err := response.BindJSONStrict(c, &req); err != nil {
		response.BadRequest(c, "Invalid request")
		return
	}
	result, err := h.service.SendInvite(c.Request.Context(), providerID, req)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}

// Consume 使用一次 Codex 重置机会。
func (h *CodexInviteResetHandler) Consume(c *gin.Context) {
	providerID, ok := parseCodexInviteResetProviderID(c)
	if !ok {
		return
	}
	// 空对象让上游选择一次可用机会。显式的空值和未知字段不能触发消费。
	var req map[string]any
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	if err := response.BindJSONStrict(c, &req); err != nil || req == nil || len(req) > 1 {
		response.BadRequest(c, "Invalid request")
		return
	}
	var creditID string
	if len(req) == 1 {
		creditID, ok = req["credit_id"].(string)
		if !ok || strings.TrimSpace(creditID) == "" || len(creditID) > 256 {
			response.BadRequest(c, "Invalid request")
			return
		}
	}
	result, err := h.service.Consume(c.Request.Context(), providerID, creditID)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}

func parseCodexInviteResetProviderID(c *gin.Context) (int64, bool) {
	c.Header("Cache-Control", "private, no-store")
	providerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || providerID <= 0 {
		response.BadRequest(c, "Invalid provider ID")
		return 0, false
	}
	return providerID, true
}
