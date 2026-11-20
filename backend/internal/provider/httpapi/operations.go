package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/TokenFlux/TokenRouter/internal/provider/operation"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

const (
	maxReloginSelection = 10000
	maxOperationBody    = 256 * 1024
)

// OperationHandler 提供批量重登、单账号推流和操作记录。
type OperationHandler struct {
	service *operation.Service
}

type reloginSelection struct {
	ProviderIDs []int64 `json:"provider_ids"`
}

type trafficPushRequest struct {
	GroupID int64 `json:"group_id"`
}

// NewOperationHandler 绑定已装配的任务服务。
func NewOperationHandler(service *operation.Service) *OperationHandler {
	return &OperationHandler{service: service}
}

// PreviewRelogin 只读取账号和档案，为批量确认准备结果。
func (h *OperationHandler) PreviewRelogin(c *gin.Context) {
	ids := bindReloginSelection(c)
	if ids == nil {
		return
	}
	httpx.Success(c, gin.H{"items": h.service.PreviewRelogin(c.Request.Context(), ids)})
}

// StartRelogin 返回入队任务和复核后跳过的账号。
func (h *OperationHandler) StartRelogin(c *gin.Context) {
	ids := bindReloginSelection(c)
	if ids == nil {
		return
	}
	jobs, skipped := h.service.SubmitRelogin(c.Request.Context(), ids)
	httpx.Accepted(c, gin.H{"operations": jobs, "skipped": skipped})
}

// StartTrafficPush 提交当前提供商在指定分组的一次推流。
func (h *OperationHandler) StartTrafficPush(c *gin.Context) {
	providerID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || providerID <= 0 {
		httpx.BadRequest(c, "Invalid provider ID")
		return
	}
	var req trafficPushRequest
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxOperationBody)
	if err := httpx.BindJSONStrict(c, &req); err != nil || req.GroupID <= 0 {
		httpx.BadRequest(c, "A positive group_id is required")
		return
	}
	job, err := h.service.SubmitTraffic(c.Request.Context(), providerID, req.GroupID)
	if err != nil {
		switch {
		case errors.Is(err, operation.ErrBusy):
			httpx.Error(c, http.StatusConflict, "该账号已有其他在途任务")
		default:
			if message, ok := operation.PublicMessage(err); ok {
				httpx.BadRequest(c, message)
			} else {
				httpx.InternalError(c, "无法提交推流任务")
			}
		}
		return
	}
	httpx.Accepted(c, job)
}

// Get 查询一项操作，浏览器可以在另一台应用节点查询同一 ID。
func (h *OperationHandler) Get(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		httpx.BadRequest(c, "Invalid operation ID")
		return
	}
	job, err := h.service.Get(c.Request.Context(), id.String())
	if errors.Is(err, operation.ErrNotFound) {
		httpx.NotFound(c, "未找到该任务")
		return
	}
	if err != nil {
		httpx.InternalError(c, "无法读取任务")
		return
	}
	httpx.Success(c, job)
}

// List 查询最近操作，可按提供商 ID 筛选。
func (h *OperationHandler) List(c *gin.Context) {
	var id int64
	if value, exists := c.GetQuery("provider_id"); exists {
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil || parsed <= 0 {
			httpx.BadRequest(c, "Invalid provider ID")
			return
		}
		id = parsed
	}
	jobs, err := h.service.List(c.Request.Context(), id)
	if err != nil {
		httpx.InternalError(c, "无法读取任务列表")
		return
	}
	httpx.Success(c, gin.H{"operations": jobs})
}

func bindReloginSelection(c *gin.Context) []int64 {
	var body reloginSelection
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxOperationBody)
	if err := httpx.BindJSONStrict(c, &body); err != nil || len(body.ProviderIDs) == 0 || len(body.ProviderIDs) > maxReloginSelection {
		httpx.BadRequest(c, "provider_ids must contain between 1 and 10000 IDs")
		return nil
	}
	ids := make([]int64, 0, len(body.ProviderIDs))
	seen := make(map[int64]struct{}, len(body.ProviderIDs))
	for _, id := range body.ProviderIDs {
		if id <= 0 {
			httpx.BadRequest(c, "Invalid provider ID")
			return nil
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}
