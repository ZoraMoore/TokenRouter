package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/identity/httpapi/authctx"
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/routing"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// PelicanHandler 将管理员的测试计划和用户可见的作品接到分组测试服务。
type PelicanHandler struct {
	service *routing.PelicanService
}

type pelicanPlanInput struct {
	GroupID              int64  `json:"group_id"`
	ProviderID           *int64 `json:"provider_id"`
	ModelID              string `json:"model_id"`
	QuestionKind         string `json:"question_kind"`
	QuestionName         string `json:"question_name,omitempty"`
	QuestionPrompt       string `json:"question_prompt,omitempty"`
	ExpectedAnswer       string `json:"expected_answer,omitempty"`
	ReasoningEffort      string `json:"reasoning_effort,omitempty"`
	CronExpression       string `json:"cron_expression"`
	Enabled              *bool  `json:"enabled" binding:"required"`
	SuspendOnWrongAnswer bool   `json:"suspend_on_wrong_answer,omitempty"`
}

func NewPelicanHandler(service *routing.PelicanService) *PelicanHandler {
	return &PelicanHandler{service: service}
}

// RegisterPelicanAdminRoutes 在智能运维下管理计划和复核结果。
func RegisterPelicanAdminRoutes(admin *gin.RouterGroup, h *PelicanHandler) {
	admin.GET("/pelican-showcase/settings", h.ShowcaseSettings)
	admin.PUT("/pelican-showcase/settings", h.SaveShowcaseSettings)
	tests := admin.Group("/pelican-tests")
	tests.GET("/question-defaults", h.QuestionDefaults)
	tests.GET("/groups/:id/providers", h.Providers)
	tests.GET("/plans", h.Plans)
	tests.POST("/plans", h.Save)
	tests.PUT("/plans/:id", h.Save)
	tests.DELETE("/plans/:id", h.Delete)
	tests.POST("/plans/:id/run", h.Run)
	tests.GET("/plans/:id/results", h.History)
	tests.GET("/results/:id", h.AdminResult)
	tests.PUT("/results/:id/review", h.Review)
}

// RegisterPelicanUserRoutes 绑定已登录用户的分组测试状态和作品。
func RegisterPelicanUserRoutes(users *gin.RouterGroup, h *PelicanHandler) {
	tests := users.Group("/pelican-tests", func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Next()
	})
	tests.GET("", h.Showcase)
	tests.GET("/visibility", h.Visibility)
	tests.GET("/results/:id", h.UserResult)
}

func pelicanID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.ErrorFrom(c, routing.ErrPelicanInvalid)
		return 0, false
	}
	return id, true
}

func pelicanReadByID[T any](c *gin.Context, read func(context.Context, int64) (T, error)) {
	id, ok := pelicanID(c)
	if !ok {
		return
	}
	result, err := read(c.Request.Context(), id)
	if !httpx.ErrorFrom(c, err) {
		httpx.Success(c, result)
	}
}

func (h *PelicanHandler) Providers(c *gin.Context) {
	pelicanReadByID(c, h.service.Providers)
}

// QuestionDefaults 让计划编辑器使用与执行器相同的内置题面和参考答案。
func (h *PelicanHandler) QuestionDefaults(c *gin.Context) {
	httpx.Success(c, provider.PelicanQuestionDefaults())
}

func (h *PelicanHandler) Plans(c *gin.Context) {
	plans, err := h.service.Plans(c.Request.Context())
	if !httpx.ErrorFrom(c, err) {
		httpx.Success(c, plans)
	}
}

func (h *PelicanHandler) Save(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var req pelicanPlanInput
	if err := httpx.BindJSONStrict(c, &req); err != nil {
		httpx.ErrorFrom(c, routing.ErrPelicanInvalid)
		return
	}
	plan := routing.PelicanPlan{
		GroupID: req.GroupID, ProviderID: req.ProviderID, ModelID: req.ModelID,
		QuestionName: req.QuestionName, QuestionPrompt: req.QuestionPrompt,
		ExpectedAnswer: req.ExpectedAnswer, ReasoningEffort: req.ReasoningEffort,
		QuestionKind: req.QuestionKind, CronExpression: req.CronExpression, Enabled: *req.Enabled,
		SuspendOnWrongAnswer: req.SuspendOnWrongAnswer,
	}
	if c.Param("id") != "" {
		id, ok := pelicanID(c)
		if !ok {
			return
		}
		plan.ID = id
	}
	saved, err := h.service.Save(c.Request.Context(), plan)
	if httpx.ErrorFrom(c, err) {
		return
	}
	if plan.ID == 0 {
		httpx.Created(c, saved)
		return
	}
	httpx.Success(c, saved)
}

func (h *PelicanHandler) Delete(c *gin.Context) {
	if id, ok := pelicanID(c); ok && !httpx.ErrorFrom(c, h.service.Delete(c.Request.Context(), id)) {
		httpx.Success(c, nil)
	}
}

func (h *PelicanHandler) Run(c *gin.Context) {
	if id, ok := pelicanID(c); ok && !httpx.ErrorFrom(c, h.service.Queue(c.Request.Context(), id)) {
		httpx.Accepted(c, nil)
	}
}

func (h *PelicanHandler) History(c *gin.Context) {
	pelicanReadByID(c, h.service.History)
}

func (h *PelicanHandler) AdminResult(c *gin.Context) {
	pelicanReadByID(c, h.service.AdminResult)
}

func (h *PelicanHandler) Review(c *gin.Context) {
	id, ok := pelicanID(c)
	if !ok {
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128)
	if err := httpx.BindJSONStrict(c, &req); err != nil {
		httpx.ErrorFrom(c, routing.ErrPelicanInvalid)
		return
	}
	if !httpx.ErrorFrom(c, h.service.Review(c.Request.Context(), id, req.Status)) {
		httpx.Success(c, nil)
	}
}

func (h *PelicanHandler) Showcase(c *gin.Context) {
	subject, ok := authctx.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		httpx.Unauthorized(c, "Sign in to view test results.")
		return
	}
	groups, err := h.service.Showcase(c.Request.Context(), subject.UserID)
	if !httpx.ErrorFrom(c, err) {
		httpx.Success(c, groups)
	}
}

func (h *PelicanHandler) UserResult(c *gin.Context) {
	subject, ok := authctx.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		httpx.Unauthorized(c, "Sign in to view test results.")
		return
	}
	pelicanReadByID(c, func(ctx context.Context, id int64) (*routing.PelicanArtwork, error) {
		return h.service.UserResult(ctx, subject.UserID, id)
	})
}
