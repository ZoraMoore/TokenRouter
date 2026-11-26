package gemini

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestUsageAuditDistinguishesCacheStates 检查缓存缺失、零值和数字格式的证据。
func TestUsageAuditDistinguishesCacheStates(t *testing.T) {
	var audit responseUsageAudit
	for _, frame := range []string{
		`{"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":80}}`,
		`{"usageMetadata":{"promptTokenCount":100,"candidatesTokenCount":4}}`,
		`{"usageMetadata":{"cachedContentTokenCount":0}}`,
		`{"usageMetadata":{"cachedContentTokenCount":"80"}}`,
		`{"usageMetadata":{"cachedContentTokenCount":8e1}}`,
	} {
		audit.observe([]byte(frame))
	}
	require.Equal(t, 5, audit.Frames)
	require.Equal(t, 1, audit.CacheMissing)
	require.Equal(t, 1, audit.CacheZero)
	require.Equal(t, 3, audit.CachePositive)
	require.Equal(t, 1, audit.CacheResets)
	require.Equal(t, "Number", audit.Last.Fields["cachedContentTokenCount"].Type)
	require.Equal(t, "8e1", audit.Last.Fields["cachedContentTokenCount"].Value)
}

// TestUsageAuditBoundsContent 验证诊断仅保存固定用量字段和有限样本。
func TestUsageAuditBoundsContent(t *testing.T) {
	var audit responseUsageAudit
	for range 100 {
		audit.observe([]byte(`{"candidates":[{"content":{"parts":[{"text":"private answer"}]}}],"usageMetadata":{"promptTokenCount":100,"cachedContentTokenCount":"private secret","private_field":"secret"}}`))
	}
	encoded, err := json.Marshal(audit)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "private")
	require.NotContains(t, string(encoded), "secret")
	require.LessOrEqual(t, len(audit.Samples), 8)
	require.Less(t, len(encoded), 8000)
}

// TestUsageAuditRecordsReadCompletion 区分完整读取响应与提前退出。
func TestUsageAuditRecordsReadCompletion(t *testing.T) {
	var audit responseUsageAudit
	body := auditedResponseBody{ReadCloser: io.NopCloser(strings.NewReader("data")), audit: &audit}
	data, err := io.ReadAll(&body)
	require.NoError(t, err)
	require.Equal(t, "data", string(data))
	require.True(t, audit.EOF)
	require.EqualValues(t, 4, audit.Bytes)
}
