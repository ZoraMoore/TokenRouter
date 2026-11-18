package openai

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestLenientJSONLimitAndOriginalBytes 检查规范化后的字节上限和原始字节保留，外层可包装并识别超限错误。
func TestLenientJSONLimitAndOriginalBytes(t *testing.T) {
	original := []byte{'{', '"', 'v', '"', ':', '"', '\n', '"', '}'}
	saved := append([]byte(nil), original...)
	normalized, err := NormalizeLenientJSONRequestBody(original, 14)
	require.NoError(t, err)
	require.Equal(t, `{"v":"\u000a"}`, string(normalized))
	require.Equal(t, saved, original)

	_, err = NormalizeLenientJSONRequestBody(original, 13)
	var exceeded *BodyLimitError
	require.True(t, errors.As(fmt.Errorf("request: %w", err), &exceeded))
	require.Equal(t, int64(13), exceeded.Limit)
}
