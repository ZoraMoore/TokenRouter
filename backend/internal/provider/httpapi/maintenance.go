package httpapi

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// MaintenanceHandler 提供管理员的计划编辑、预览和运行记录。
type MaintenanceHandler struct {
	service *provider.MaintenanceService
}

func NewMaintenanceHandler(service *provider.MaintenanceService) *MaintenanceHandler {
	return &MaintenanceHandler{service: service}
}

// RegisterMaintenanceRoutes 在管理员路由下挂载定时任务。
func RegisterMaintenanceRoutes(admin *gin.RouterGroup, h *MaintenanceHandler) {
	tasks := admin.Group("/scheduled-tasks", func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		c.Next()
	})
	tasks.GET("/plans", h.plans)
	tasks.POST("/plans", h.save)
	tasks.PUT("/plans/:id", h.save)
	tasks.POST("/preview", h.preview)
	tasks.POST("/iq-rechecks", h.recheck)
	tasks.POST("/plans/:id/run", h.runNow)
	tasks.GET("/plans/:id/runs", h.runs)
	tasks.GET("/runs/:id", h.run)
}

func (h *MaintenanceHandler) plans(c *gin.Context) {
	plans, err := h.service.Plans(c.Request.Context())
	if !httpx.ErrorFrom(c, err) {
		httpx.Success(c, gin.H{"items": plans, "timezone": h.service.Timezone()})
	}
}

func (h *MaintenanceHandler) save(c *gin.Context) {
	var id int64
	if c.Param("id") != "" {
		var ok bool
		id, ok = maintenanceID(c)
		if !ok {
			return
		}
	}
	draft, ok := maintenanceDraft(c)
	if !ok {
		return
	}
	plan, err := h.service.Save(c.Request.Context(), id, draft)
	if httpx.ErrorFrom(c, err) {
		return
	}
	if id == 0 {
		httpx.Created(c, plan)
		return
	}
	httpx.Success(c, plan)
}

func (h *MaintenanceHandler) preview(c *gin.Context) {
	draft, ok := maintenanceDraft(c)
	if !ok {
		return
	}
	preview, err := h.service.Preview(c.Request.Context(), draft)
	if !httpx.ErrorFrom(c, err) {
		httpx.Success(c, preview)
	}
}

func (h *MaintenanceHandler) runNow(c *gin.Context) {
	id, ok := maintenanceID(c)
	if !ok {
		return
	}
	runID, err := h.service.RunNow(c.Request.Context(), id)
	if !httpx.ErrorFrom(c, err) {
		httpx.Accepted(c, gin.H{"run_id": runID})
	}
}

// recheck 按已落库的分组样本返回定向复检的运行编号。
func (h *MaintenanceHandler) recheck(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var request struct {
		ResultID int64 `json:"pelican_result_id" binding:"gt=0"`
	}
	if err := httpx.BindJSONStrict(c, &request); err != nil {
		httpx.ErrorFrom(c, provider.ErrMaintenanceInvalid)
		return
	}
	runID, err := h.service.Recheck(c.Request.Context(), request.ResultID)
	if !httpx.ErrorFrom(c, err) {
		httpx.Accepted(c, gin.H{"run_id": runID})
	}
}

func (h *MaintenanceHandler) runs(c *gin.Context) {
	maintenanceRead(c, func(ctx context.Context, id int64) (gin.H, error) {
		runs, err := h.service.Runs(ctx, id)
		return gin.H{"items": runs}, err
	})
}

func (h *MaintenanceHandler) run(c *gin.Context) {
	maintenanceRead(c, h.service.Run)
}

func maintenanceRead[T any](c *gin.Context, read func(context.Context, int64) (T, error)) {
	id, ok := maintenanceID(c)
	if !ok {
		return
	}
	data, err := read(c.Request.Context(), id)
	if !httpx.ErrorFrom(c, err) {
		httpx.Success(c, data)
	}
}

func maintenanceDraft(c *gin.Context) (provider.MaintenanceDraft, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var draft provider.MaintenanceDraft
	err := httpx.BindJSONStrict(c, &draft)
	if err != nil {
		httpx.ErrorFrom(c, provider.ErrMaintenanceInvalid)
	}
	return draft, err == nil
}

func maintenanceID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.ErrorFrom(c, provider.ErrMaintenanceInvalid)
		return 0, false
	}
	return id, true
}
