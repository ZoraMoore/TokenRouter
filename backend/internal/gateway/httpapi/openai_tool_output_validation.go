package httpapi

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"

	"github.com/TokenFlux/TokenRouter/internal/protocol/openai"
)

func ValidateOpenAIFunctionCallOutput(c *gin.Context, body []byte, reqLog *zap.Logger) bool {
	if !gjson.GetBytes(body, `input.#(type=="function_call_output")`).Exists() {
		return true
	}

	validation := openai.ValidateFunctionCallOutputContextBytes(body)
	if !validation.HasFunctionCallOutput {
		return true
	}

	previousResponseID := gjson.GetBytes(body, "previous_response_id").String()
	if strings.TrimSpace(previousResponseID) != "" || validation.HasToolCallContext {
		return true
	}

	if validation.HasFunctionCallOutputMissingCallID {
		reqLog.Warn("openai.request_validation_failed",
			zap.String("reason", "function_call_output_missing_call_id"),
			zap.Int64("input_index", validation.MissingCallIDInputIndex),
			zap.String("item_type", validation.MissingCallIDItemType),
		)
		message := fmt.Sprintf("input[%d] (%s) requires call_id", validation.MissingCallIDInputIndex, validation.MissingCallIDItemType)
		DefaultOpenAIErrorOutput().WriteError(c, http.StatusBadRequest, "invalid_request_error", message)
		return false
	}
	if validation.HasItemReferenceForAllCallIDs {
		return true
	}

	reqLog.Warn("openai.request_validation_failed",
		zap.String("reason", "function_call_output_missing_item_reference"),
	)
	DefaultOpenAIErrorOutput().WriteError(c, http.StatusBadRequest, "invalid_request_error", "tool output requires matching tool call context or item references")
	return false
}
