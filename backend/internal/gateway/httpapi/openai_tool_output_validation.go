package httpapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"github.com/TokenFlux/TokenRouter/internal/gateway/requeststate"
	"github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

const codexDesktopContextKey = "openai.desktop_context"

var toolLogName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// HasCodexDesktopContext 表示请求入站时已识别出独立桌面上下文。
func HasCodexDesktopContext(c *gin.Context) bool { return c.GetBool(codexDesktopContextKey) }

func ValidateOpenAIFunctionCallOutput(c *gin.Context, body []byte, reqLog *zap.Logger) bool {
	validation := openai.ValidateFunctionCallOutputContextBytes(body)
	if validation.HasDesktopContext && !requeststate.HasUniqueJSONMembers(body) {
		DefaultOpenAIErrorOutput().WriteError(c, http.StatusBadRequest, "invalid_request_error", "ambiguous duplicate JSON members in desktop context request")
		return false
	}
	if validation.HasDesktopContext {
		c.Set(codexDesktopContextKey, true)
	}
	if !validation.HasFunctionCallOutput {
		return true
	}
	if validation.HasFunctionCallOutputMissingCallID {
		item := gjson.GetBytes(body, fmt.Sprintf("input.%d", validation.MissingCallIDInputIndex))
		id := item.Get("call_id")
		idState := "missing"
		switch {
		case id.Type == gjson.String && strings.TrimSpace(id.Str) == "":
			idState = "blank"
		case id.Exists() && id.Type == gjson.Null:
			idState = "null"
		case id.Exists():
			idState = "invalid_type"
		}
		details := []zap.Field{zap.String("call_id_state", idState)}
		for _, key := range []string{"namespace", "name"} {
			v := item.Get(key)
			if v.Type == gjson.String && toolLogName.MatchString(v.Str) {
				details = append(details, zap.String("item_"+key, v.Str))
			}
		}
		reqLog.With(details...).Warn("openai.request_validation_failed",
			zap.String("reason", "function_call_output_missing_call_id"),
			zap.Int64("input_index", validation.MissingCallIDInputIndex),
			zap.String("item_type", validation.MissingCallIDItemType),
		)
		message := fmt.Sprintf("input[%d] (%s) requires call_id", validation.MissingCallIDInputIndex, validation.MissingCallIDItemType)
		DefaultOpenAIErrorOutput().WriteError(c, http.StatusBadRequest, "invalid_request_error", message)
		return false
	}
	if validation.HasItemReferenceForAllCallIDs || strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").Str) != "" {
		return true
	}

	reqLog.Warn("openai.request_validation_failed",
		zap.String("reason", "function_call_output_missing_item_reference"),
		zap.Int64("input_index", validation.UnmatchedInputIndex),
	)
	DefaultOpenAIErrorOutput().WriteError(c, http.StatusBadRequest, "invalid_request_error", fmt.Sprintf("input[%d] tool output requires matching tool call context or item references", validation.UnmatchedInputIndex))
	return false
}
