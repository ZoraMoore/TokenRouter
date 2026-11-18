package batchimage

import (
	"fmt"
	"strings"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

var (
	ErrBatchImageProviderUnsupportedProvider     = apperror.New(apperror.CategoryBadRequest, "BATCH_IMAGE_PROVIDER_UNSUPPORTED_PROVIDER", "batch image platform does not support this provider")
	ErrBatchImageProviderMissingAPIKey           = apperror.New(apperror.CategoryBadRequest, "BATCH_IMAGE_PROVIDER_MISSING_API_KEY", "batch image platform provider is missing api key")
	ErrBatchImageProviderMissingServiceAccount   = apperror.New(apperror.CategoryBadRequest, "BATCH_IMAGE_PROVIDER_MISSING_SERVICE_ACCOUNT", "batch image platform provider is missing service account credentials")
	ErrBatchImageProviderMissingJobName          = apperror.New(apperror.CategoryBadRequest, "BATCH_IMAGE_PROVIDER_MISSING_JOB_NAME", "batch image platform job name is missing")
	ErrBatchImageProviderMissingResultRef        = apperror.New(apperror.CategoryBadRequest, "BATCH_IMAGE_PROVIDER_MISSING_RESULT_REF", "batch image platform result reference is missing")
	ErrBatchImageProviderInlineResultUnsupported = apperror.New(apperror.CategoryBadRequest, "GEMINI_INLINE_BATCH_RESULT_UNSUPPORTED", "Gemini inline batch result is not supported")
	ErrBatchImageProviderInvalidInput            = apperror.New(apperror.CategoryBadRequest, "BATCH_IMAGE_PROVIDER_INVALID_INPUT", "invalid batch image platform input")
	ErrBatchImageProviderUnsafeCleanupPath       = apperror.New(apperror.CategoryBadRequest, "VERTEX_UNSAFE_CLEANUP_PATH", "unsafe batch image cleanup path")
	ErrUnsupportedCleanupTarget                  = apperror.New(apperror.CategoryBadRequest, "BATCH_IMAGE_PROVIDER_UNSUPPORTED_CLEANUP_TARGET", "unsupported batch image cleanup target")
)

func BatchImageProviderJobName(job *BatchImageJob) string {
	if job == nil || job.ProviderJobName == nil {
		return ""
	}
	return strings.TrimSpace(*job.ProviderJobName)
}

func BatchImageProviderInputRef(job *BatchImageJob) string {
	if job == nil || job.ProviderInputRef == nil {
		return ""
	}
	return strings.TrimSpace(*job.ProviderInputRef)
}

func BatchImageProviderOutputRef(job *BatchImageJob) string {
	if job == nil || job.ProviderOutputRef == nil {
		return ""
	}
	return strings.TrimSpace(*job.ProviderOutputRef)
}

func BatchImageProviderInputError(format string, args ...any) error {
	return ErrBatchImageProviderInvalidInput.WithCause(fmt.Errorf(format, args...))
}
