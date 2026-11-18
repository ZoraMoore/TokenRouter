package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/egress"
	"github.com/TokenFlux/TokenRouter/internal/egress/provider"
	response "github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// TLSFingerprintProfileHandler 处理 TLS 指纹模板的 HTTP 请求。
type TLSFingerprintProfileHandler struct {
	service              *egress.TLSFingerprintProfileService
	collector            egress.Collector
	authenticatedIngress *provider.AuthenticatedIngressTLSService
}

// CreateTLSFingerprintProfileRequest 创建模板请求。
type CreateTLSFingerprintProfileRequest struct {
	Name                    string   `json:"name" binding:"required"`
	Description             *string  `json:"description"`
	EnableGREASE            *bool    `json:"enable_grease"`
	CipherSuites            []uint16 `json:"cipher_suites"`
	Curves                  []uint16 `json:"curves"`
	PointFormats            []uint16 `json:"point_formats"`
	SignatureAlgorithms     []uint16 `json:"signature_algorithms"`
	SignatureAlgorithmsCert []uint16 `json:"signature_algorithms_cert"`
	ALPNProtocols           []string `json:"alpn_protocols"`
	SupportedVersions       []uint16 `json:"supported_versions"`
	KeyShareGroups          []uint16 `json:"key_share_groups"`
	PSKModes                []uint16 `json:"psk_modes"`
	Extensions              []uint16 `json:"extensions"`
}

// UpdateTLSFingerprintProfileRequest 更新模板请求（部分更新）。
type UpdateTLSFingerprintProfileRequest struct {
	Name                    *string  `json:"name"`
	Description             *string  `json:"description"`
	EnableGREASE            *bool    `json:"enable_grease"`
	CipherSuites            []uint16 `json:"cipher_suites"`
	Curves                  []uint16 `json:"curves"`
	PointFormats            []uint16 `json:"point_formats"`
	SignatureAlgorithms     []uint16 `json:"signature_algorithms"`
	SignatureAlgorithmsCert []uint16 `json:"signature_algorithms_cert"`
	ALPNProtocols           []string `json:"alpn_protocols"`
	SupportedVersions       []uint16 `json:"supported_versions"`
	KeyShareGroups          []uint16 `json:"key_share_groups"`
	PSKModes                []uint16 `json:"psk_modes"`
	Extensions              []uint16 `json:"extensions"`
}

// PromoteTLSFingerprintCaptureRequest 允许管理员覆盖已验证模板的展示字段。
// 握手字段和验证证据始终由服务端从受控采集记录生成。
type PromoteTLSFingerprintCaptureRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

// SetTLSFingerprintProfileApprovalRequest 控制已验证模板是否可绑定及是否进入随机池。
type SetTLSFingerprintProfileApprovalRequest struct {
	ApprovedForUse     bool `json:"approved_for_use"`
	RandomPoolEligible bool `json:"random_pool_eligible"`
}

// NewTLSFingerprintProfileHandler 创建 TLS 指纹模板处理器。
func NewTLSFingerprintProfileHandler(profileService *egress.TLSFingerprintProfileService, collectors ...egress.Collector) *TLSFingerprintProfileHandler {
	h := &TLSFingerprintProfileHandler{service: profileService}
	if len(collectors) > 0 {
		h.collector = collectors[0]
	}
	return h
}

// SetAuthenticatedIngressTLSService 注入无需客户配置的已认证入站采集状态服务。
func (h *TLSFingerprintProfileHandler) SetAuthenticatedIngressTLSService(value *provider.AuthenticatedIngressTLSService) {
	h.authenticatedIngress = value
}

// List 获取所有模板
// GET /api/v1/admin/tls-fingerprint-profiles
func (h *TLSFingerprintProfileHandler) List(c *gin.Context) {
	var (
		profiles []*egress.TLSFingerprintProfile
		err      error
	)
	if c.Query("bindable") == "true" {
		profiles, err = h.service.ListBindable(c.Request.Context(), c.Query("target_host"))
	} else {
		profiles, err = h.service.List(c.Request.Context())
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, profiles)
}

// GetByID 根据 ID 获取模板
// GET /api/v1/admin/tls-fingerprint-profiles/:id
func (h *TLSFingerprintProfileHandler) GetByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid profile ID")
		return
	}

	profile, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if profile == nil {
		response.NotFound(c, "Profile not found")
		return
	}

	response.Success(c, profile)
}

// Create 创建模板
// POST /api/v1/admin/tls-fingerprint-profiles
func (h *TLSFingerprintProfileHandler) Create(c *gin.Context) {
	var req CreateTLSFingerprintProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	profile := &egress.TLSFingerprintProfile{
		Name:                    req.Name,
		Description:             req.Description,
		CipherSuites:            req.CipherSuites,
		Curves:                  req.Curves,
		PointFormats:            req.PointFormats,
		SignatureAlgorithms:     req.SignatureAlgorithms,
		SignatureAlgorithmsCert: req.SignatureAlgorithmsCert,
		ALPNProtocols:           req.ALPNProtocols,
		SupportedVersions:       req.SupportedVersions,
		KeyShareGroups:          req.KeyShareGroups,
		PSKModes:                req.PSKModes,
		Extensions:              req.Extensions,
	}

	if req.EnableGREASE != nil {
		profile.EnableGREASE = *req.EnableGREASE
	}

	created, err := h.service.Create(c.Request.Context(), profile)
	if err != nil {
		if _, ok := err.(*egress.ValidationError); ok {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, created)
}

// Update 更新模板（支持部分更新）
// PUT /api/v1/admin/tls-fingerprint-profiles/:id
func (h *TLSFingerprintProfileHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid profile ID")
		return
	}

	var req UpdateTLSFingerprintProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	existing, err := h.service.GetByID(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if existing == nil {
		response.NotFound(c, "Profile not found")
		return
	}

	// 部分更新
	profile := &egress.TLSFingerprintProfile{
		ID:                      id,
		Name:                    existing.Name,
		Description:             existing.Description,
		EnableGREASE:            existing.EnableGREASE,
		CipherSuites:            existing.CipherSuites,
		Curves:                  existing.Curves,
		PointFormats:            existing.PointFormats,
		SignatureAlgorithms:     existing.SignatureAlgorithms,
		SignatureAlgorithmsCert: existing.SignatureAlgorithmsCert,
		ALPNProtocols:           existing.ALPNProtocols,
		SupportedVersions:       existing.SupportedVersions,
		KeyShareGroups:          existing.KeyShareGroups,
		PSKModes:                existing.PSKModes,
		Extensions:              existing.Extensions,
		Verification:            existing.Verification,
	}

	if req.Name != nil {
		profile.Name = *req.Name
	}
	if req.Description != nil {
		profile.Description = req.Description
	}
	if req.EnableGREASE != nil {
		profile.EnableGREASE = *req.EnableGREASE
	}
	if req.CipherSuites != nil {
		profile.CipherSuites = req.CipherSuites
	}
	if req.Curves != nil {
		profile.Curves = req.Curves
	}
	if req.PointFormats != nil {
		profile.PointFormats = req.PointFormats
	}
	if req.SignatureAlgorithms != nil {
		profile.SignatureAlgorithms = req.SignatureAlgorithms
	}
	if req.SignatureAlgorithmsCert != nil {
		profile.SignatureAlgorithmsCert = req.SignatureAlgorithmsCert
	}
	if req.ALPNProtocols != nil {
		profile.ALPNProtocols = req.ALPNProtocols
	}
	if req.SupportedVersions != nil {
		profile.SupportedVersions = req.SupportedVersions
	}
	if req.KeyShareGroups != nil {
		profile.KeyShareGroups = req.KeyShareGroups
	}
	if req.PSKModes != nil {
		profile.PSKModes = req.PSKModes
	}
	if req.Extensions != nil {
		profile.Extensions = req.Extensions
	}

	updated, err := h.service.Update(c.Request.Context(), profile)
	if err != nil {
		if _, ok := err.(*egress.ValidationError); ok {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, updated)
}

// Delete 删除模板
// DELETE /api/v1/admin/tls-fingerprint-profiles/:id
func (h *TLSFingerprintProfileHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid profile ID")
		return
	}

	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, gin.H{"message": "Profile deleted successfully"})
}

// SetApproval 显式批准或撤回自动生成的已验证模板。
// POST /api/v1/admin/tls-fingerprint-profiles/:id/approval
func (h *TLSFingerprintProfileHandler) SetApproval(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		response.BadRequest(c, "Invalid profile ID")
		return
	}
	var req SetTLSFingerprintProfileApprovalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	updated, err := h.service.SetApproval(c.Request.Context(), id, req.ApprovedForUse, req.RandomPoolEligible)
	if err != nil {
		if _, ok := err.(*egress.ValidationError); ok {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}

// CollectorStatus 获取 TLS 指纹收集器状态
// GET /api/v1/admin/tls-fingerprint-profiles/collector/status
func (h *TLSFingerprintProfileHandler) CollectorStatus(c *gin.Context) {
	if h.collector == nil {
		response.Success(c, provider.TLSFingerprintCollectorStatus{})
		return
	}
	response.Success(c, h.collector.Status())
}

// AuthenticatedIngressStatus 返回不包含客户、Key、地址或 flow 标识的采集状态。
func (h *TLSFingerprintProfileHandler) AuthenticatedIngressStatus(c *gin.Context) {
	if h.authenticatedIngress == nil {
		response.Success(c, provider.AuthenticatedIngressTLSStatus{})
		return
	}
	response.Success(c, h.authenticatedIngress.Status())
}

// StartCollector 从页面启动 TLS 指纹收集器
// POST /api/v1/admin/tls-fingerprint-profiles/collector/start
func (h *TLSFingerprintProfileHandler) StartCollector(c *gin.Context) {
	if h.collector == nil {
		response.Error(c, http.StatusServiceUnavailable, "TLS fingerprint collector is unavailable")
		return
	}
	status, err := h.collector.Start(c.Request.Context())
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, status)
}

// StopCollector 从页面停止 TLS 指纹收集器
// POST /api/v1/admin/tls-fingerprint-profiles/collector/stop
func (h *TLSFingerprintProfileHandler) StopCollector(c *gin.Context) {
	if h.collector == nil {
		response.Error(c, http.StatusServiceUnavailable, "TLS fingerprint collector is unavailable")
		return
	}
	if err := h.collector.Stop(c.Request.Context()); err != nil {
		response.Error(c, http.StatusInternalServerError, err.Error())
		return
	}
	response.Success(c, h.collector.Status())
}

// CreateCollectorSession 创建一次短期采集会话
// POST /api/v1/admin/tls-fingerprint-profiles/collector/sessions
func (h *TLSFingerprintProfileHandler) CreateCollectorSession(c *gin.Context) {
	if h.collector == nil {
		response.Error(c, http.StatusServiceUnavailable, "TLS fingerprint collector is unavailable")
		return
	}
	var req provider.TLSFingerprintCollectorSessionRequest
	if c.Request != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid request: "+err.Error())
			return
		}
	}
	session, err := h.createControlledSession(req)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, session)
}

// PromoteCollectorCapture 验证受控采集并创建可绑定模板。
// POST /api/v1/admin/tls-fingerprint-profiles/collector/sessions/:token/captures/:capture_id/promote
func (h *TLSFingerprintProfileHandler) PromoteCollectorCapture(c *gin.Context) {
	if h.collector == nil {
		response.Error(c, http.StatusServiceUnavailable, "TLS fingerprint collector is unavailable")
		return
	}
	var req PromoteTLSFingerprintCaptureRequest
	if c.Request != nil && c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			response.BadRequest(c, "Invalid request: "+err.Error())
			return
		}
	}
	profile, err := h.buildVerifiedProfile(c.Request.Context(), c.Param("token"), c.Param("capture_id"))
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.Name != nil {
		if name := strings.TrimSpace(*req.Name); name != "" {
			profile.Name = name
		}
	}
	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		profile.Description = &description
	}
	created, err := h.service.Create(c.Request.Context(), profile)
	if err != nil {
		if _, ok := err.(*egress.ValidationError); ok {
			response.BadRequest(c, err.Error())
			return
		}
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, created)
}

// ListCollectorCaptures 获取采集会话结果
// GET /api/v1/admin/tls-fingerprint-profiles/collector/sessions/:token/captures
func (h *TLSFingerprintProfileHandler) ListCollectorCaptures(c *gin.Context) {
	if h.collector == nil {
		response.Error(c, http.StatusServiceUnavailable, "TLS fingerprint collector is unavailable")
		return
	}
	records, err := h.collector.ListCaptures(c.Param("token"))
	if err != nil {
		response.NotFound(c, err.Error())
		return
	}
	response.Success(c, records)
}

// DeleteCollectorSession 删除采集会话
// DELETE /api/v1/admin/tls-fingerprint-profiles/collector/sessions/:token
func (h *TLSFingerprintProfileHandler) DeleteCollectorSession(c *gin.Context) {
	if h.collector == nil {
		response.Error(c, http.StatusServiceUnavailable, "TLS fingerprint collector is unavailable")
		return
	}
	h.collector.DeleteSession(c.Param("token"))
	response.Success(c, gin.H{"deleted": true})
}

// createControlledSession 从管理请求建立带客户端声明的采集会话。
func (h *TLSFingerprintProfileHandler) createControlledSession(req provider.TLSFingerprintCollectorSessionRequest) (*egress.TLSFingerprintCollectorSession, error) {
	collector, ok := h.collector.(interface {
		CreateControlledSession(egress.TLSFingerprintCollectorSessionRequest) (*egress.TLSFingerprintCollectorSession, error)
	})
	if !ok {
		return nil, fmt.Errorf("controlled collector is unavailable")
	}
	return collector.CreateControlledSession(req)
}

// buildVerifiedProfile 验证采集记录并生成带来源证据的模板。
func (h *TLSFingerprintProfileHandler) buildVerifiedProfile(ctx context.Context, token, id string) (*egress.TLSFingerprintProfile, error) {
	collector, ok := h.collector.(interface {
		BuildVerifiedProfile(context.Context, string, string) (*egress.TLSFingerprintProfile, error)
	})
	if !ok {
		return nil, fmt.Errorf("verified collector is unavailable")
	}
	return collector.BuildVerifiedProfile(ctx, token, id)
}
