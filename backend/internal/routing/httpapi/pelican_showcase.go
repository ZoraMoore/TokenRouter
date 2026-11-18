package httpapi

import (
	"net/http"
	"path"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

type pelicanSettingsInput struct {
	Enabled       *bool `json:"enabled" binding:"required"`
	APIEnabled    *bool `json:"api_enabled" binding:"required"`
	MaxItems      int   `json:"max_items"`
	AutoCleanup   *bool `json:"auto_cleanup" binding:"required"`
	RetentionDays int   `json:"retention_days"`
}

func (h *PelicanHandler) ShowcaseSettings(c *gin.Context) {
	settings, err := h.service.ShowcaseSettings(c.Request.Context())
	if !httpx.ErrorFrom(c, err) {
		httpx.Success(c, settings)
	}
}

func (h *PelicanHandler) SaveShowcaseSettings(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var input pelicanSettingsInput
	if err := httpx.BindJSONStrict(c, &input); err != nil {
		httpx.ErrorFrom(c, routing.ErrPelicanInvalid)
		return
	}
	settings, err := h.service.SaveShowcaseSettings(c.Request.Context(), routing.PelicanShowcaseSettings{
		Enabled: *input.Enabled, APIEnabled: *input.APIEnabled, AutoCleanup: *input.AutoCleanup,
		MaxItems: input.MaxItems, RetentionDays: input.RetentionDays,
	})
	if !httpx.ErrorFrom(c, err) {
		httpx.Success(c, settings)
	}
}

// Visibility 供登录后的菜单和展示页读取当前有效的开关。
func (h *PelicanHandler) Visibility(c *gin.Context) {
	subject, ok := authctx.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		httpx.Unauthorized(c, "Sign in to view test results.")
		return
	}
	flags, err := h.service.Visibility(c.Request.Context())
	if !httpx.ErrorFrom(c, err) {
		httpx.Success(c, flags)
	}
}

// RegisterPelicanKeyRoutes 使用网关已认证的 Key 范围读取作品，同一个范围适用于 GET 和 HEAD。
func RegisterPelicanKeyRoutes(group *gin.RouterGroup, h *PelicanHandler, scope func(*gin.Context) (int64, []int64, bool)) {
	itemPath := path.Join(group.BasePath(), "items") + "/"
	read := func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		if scope != nil {
			if userID, bound, ok := scope(c); ok && userID > 0 {
				data, err := h.keyShowcase(c, userID, bound, itemPath)
				writePelicanKeyResponse(c, data, err)
				return
			}
		}
		writePelicanKeyResponse(c, nil, apperror.Unauthorized("UNAUTHORIZED", "A valid API key is required."))
	}
	group.GET("", read)
	group.HEAD("", read)
	group.GET("/items/:id", read)
	group.HEAD("/items/:id", read)
}

func (h *PelicanHandler) keyShowcase(c *gin.Context, userID int64, bound []int64, itemPath string) (any, error) {
	raw := c.Param("id")
	if raw == "" {
		view, err := h.service.KeyShowcase(c.Request.Context(), userID, bound)
		if err != nil {
			return nil, err
		}
		for _, group := range view.Groups {
			for i := range group.Items {
				item := &group.Items[i]
				item.ContentURL = itemPath + strconv.FormatInt(item.ID, 10)
			}
		}
		return view, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return nil, routing.ErrPelicanInvalid
	}
	item, err := h.service.KeyArtwork(c.Request.Context(), userID, bound, id)
	if err != nil {
		return nil, err
	}
	item.ContentURL = itemPath + strconv.FormatInt(item.ID, 10)
	return item, nil
}

// writePelicanKeyResponse 在 HEAD 上保留 GET 的访问检查和状态码，省去响应正文。
func writePelicanKeyResponse(c *gin.Context, data any, err error) {
	if c.Request.Method == http.MethodHead {
		status, _ := httpx.ToHTTP(err)
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.Status(status)
		return
	}
	if !httpx.ErrorFrom(c, err) {
		httpx.Success(c, data)
	}
}
