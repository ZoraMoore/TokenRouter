package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/server/httpx"
)

// iqStreamEvent 同时用于题目、样本及整轮结果。
type iqStreamEvent struct {
	Type string `json:"type"`
	Data any    `json:"data,omitempty"`
}

// IQQuestion 将题面发给选定的账号，返回该次回答和耗时。
func (h *TestHandler) IQQuestion(c *gin.Context) {
	id, req, ok := bindIQTest(c, true)
	if !ok {
		return
	}
	result, err := h.tests.RunIQQuestion(c.Request.Context(), id, req)
	if err != nil {
		writeIQError(c, err)
		return
	}
	httpx.Success(c, result)
}

// IQQuality 返回按标准答案核对的样本和每题的判定。
func (h *TestHandler) IQQuality(c *gin.Context) {
	id, req, ok := bindIQTest(c, false)
	if !ok {
		return
	}
	result, err := h.tests.RunIQQuality(c.Request.Context(), id, req)
	if err != nil {
		writeIQError(c, err)
		return
	}
	httpx.Success(c, result)
}

// StateProbe 只接收模型名，两次协议请求的认证和状态由服务端管理。
func (h *TestHandler) StateProbe(c *gin.Context) {
	id, ok := iqProviderID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var options map[string]json.RawMessage
	if err := httpx.BindJSONStrict(c, &options); err != nil || options == nil {
		httpx.BadRequest(c, "探针请求需要包含模型名的 JSON 对象，使用默认模型可传空对象")
		return
	}
	model := ""
	if len(options) > 0 {
		raw, ok := options["model_id"]
		if !ok || len(options) != 1 {
			httpx.BadRequest(c, "探针请求只接受 model_id 字段")
			return
		}
		var selected *string
		if err := json.Unmarshal(raw, &selected); err != nil || selected == nil || len(*selected) > 255 {
			httpx.BadRequest(c, "模型名必须是最多 255 字节的字符串")
			return
		}
		model = *selected
	}
	result, err := h.tests.RunStateProbe(c.Request.Context(), id, model)
	if err != nil {
		writeIQError(c, err)
		return
	}
	httpx.Success(c, result)
}

// ModelTrace 逐题推送指纹结果。首个事件到达前仍可返回账号不适用或忙碌的 HTTP 状态。
func (h *TestHandler) ModelTrace(c *gin.Context) {
	id, req, ok := bindIQTest(c, false)
	if !ok {
		return
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	events := make(chan []byte)
	done := make(chan error, 1)
	// 请求协程独占响应写入，耗时题目执行期间也能发送 SSE 保活。
	go func() {
		done <- h.tests.RunModelTrace(ctx, id, req, func(name string, data any) error {
			raw, err := json.Marshal(iqStreamEvent{Type: name, Data: data})
			if err != nil {
				return err
			}
			select {
			case events <- raw:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()
	started := false
	for {
		select {
		case <-ctx.Done():
			return
		case err := <-done:
			if err != nil && ctx.Err() == nil {
				if started {
					_ = writeIQEvent(c, []byte(`{"type":"error"}`))
				} else {
					writeIQError(c, err)
				}
			}
			return
		case event := <-events:
			if !started {
				c.Header("Content-Type", "text/event-stream")
				c.Header("X-Accel-Buffering", "no")
				started = true
			}
			if err := writeIQEvent(c, event); err != nil {
				return
			}
		case <-heartbeat.C:
			if started {
				if _, err := c.Writer.WriteString(": keep-alive\n\n"); err != nil {
					return
				}
				c.Writer.Flush()
			}
		}
	}
}

func writeIQEvent(c *gin.Context, data []byte) error {
	if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

// iqProviderID 将管理员的测试请求绑定到路由中的账号。
func iqProviderID(c *gin.Context) (int64, bool) {
	c.Header("Cache-Control", "private, no-store")
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		httpx.BadRequest(c, "提供商 ID 无效")
		return 0, false
	}
	return id, true
}

// bindIQTest 接收答题参数，质量检测和指纹各自生成题面。
func bindIQTest(c *gin.Context, customQuestion bool) (int64, provider.IQTestRequest, bool) {
	var req provider.IQTestRequest
	id, ok := iqProviderID(c)
	if !ok {
		return 0, req, false
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var err error
	if customQuestion {
		err = httpx.BindJSONStrict(c, &req)
	} else {
		var options struct {
			ModelID         string `json:"model_id" binding:"required"`
			ReasoningEffort string `json:"reasoning_effort"`
		}
		err = httpx.BindJSONStrict(c, &options)
		req.ModelID = options.ModelID
		req.ReasoningEffort = options.ReasoningEffort
	}
	if err != nil {
		httpx.BadRequest(c, "智商测试请求格式有误")
		return 0, req, false
	}
	return id, req, true
}

// writeIQError 返回可供管理员处理的错误类别，上游响应可能包含账号信息。
func writeIQError(c *gin.Context, err error) {
	code, reason, message := http.StatusBadGateway, "IQ_TEST_FAILED", "账号测试未能完成"
	switch {
	case errors.Is(err, provider.ErrIQBusy):
		code, reason, message = http.StatusConflict, "IQ_TEST_BUSY", "这个账号的测试正在运行，请稍后重试"
	case errors.Is(err, provider.ErrIQUnsupported):
		code, reason, message = http.StatusBadRequest, "IQ_TEST_UNSUPPORTED", "这个账号或模型不支持所选测试"
	case errors.Is(err, provider.ErrIQInvalid):
		code, reason, message = http.StatusBadRequest, "IQ_TEST_INVALID", "请检查模型、推理强度和题目设置"
	case errors.Is(err, provider.ErrIQNotFound):
		code, reason, message = http.StatusNotFound, "IQ_TEST_NOT_FOUND", "找不到这个提供商"
	case errors.Is(err, provider.ErrIQUnavailable):
		code, reason, message = http.StatusServiceUnavailable, "IQ_TEST_UNAVAILABLE", "暂时无法读取这个提供商"
	case errors.Is(err, context.DeadlineExceeded):
		code, reason, message = http.StatusGatewayTimeout, "IQ_TEST_TIMEOUT", "等待账号答题超时"
	}
	httpx.ErrorWithDetails(c, code, message, reason, nil)
}
