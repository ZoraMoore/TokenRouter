package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestExtractGeminiCLISessionHash(t *testing.T) {
	tests := []struct {
		name             string
		body             string
		privilegedUserID string
		wantEmpty        bool
		wantHash         string
	}{
		{
			name:             "with privileged-user-id and tmp dir",
			body:             `{"contents":[{"parts":[{"text":"The project's temporary directory is: /Users/ianshaw/.gemini/tmp/f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740"}]}]}`,
			privilegedUserID: "90785f52-8bbe-4b17-b111-a1ddea1636c3",
			wantEmpty:        false,
			wantHash: func() string {
				combined := "90785f52-8bbe-4b17-b111-a1ddea1636c3:f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740"
				hash := sha256.Sum256([]byte(combined))
				return hex.EncodeToString(hash[:])
			}(),
		},
		{
			name:             "without privileged-user-id but with tmp dir",
			body:             `{"contents":[{"parts":[{"text":"The project's temporary directory is: /Users/ianshaw/.gemini/tmp/f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740"}]}]}`,
			privilegedUserID: "",
			wantEmpty:        false,
			wantHash:         "f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740",
		},
		{
			name:             "without tmp dir",
			body:             `{"contents":[{"parts":[{"text":"Hello world"}]}]}`,
			privilegedUserID: "90785f52-8bbe-4b17-b111-a1ddea1636c3",
			wantEmpty:        true,
		},
		{
			name:             "empty body",
			body:             "",
			privilegedUserID: "90785f52-8bbe-4b17-b111-a1ddea1636c3",
			wantEmpty:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建测试上下文
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodPost, "/test", nil)
			if tt.privilegedUserID != "" {
				c.Request.Header.Set("x-gemini-api-privileged-user-id", tt.privilegedUserID)
			}

			// 调用函数
			result := ExtractGeminiCLISessionHash(c, []byte(tt.body))

			// 验证结果
			if tt.wantEmpty {
				require.Empty(t, result, "expected empty session hash")
			} else {
				require.NotEmpty(t, result, "expected non-empty session hash")
				require.Equal(t, tt.wantHash, result, "session hash mismatch")
			}
		})
	}
}

func TestGeminiCLITmpDirRegex(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMatch bool
		wantHash  string
	}{
		{
			name:      "valid tmp dir path",
			input:     "/Users/ianshaw/.gemini/tmp/f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740",
			wantMatch: true,
			wantHash:  "f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740",
		},
		{
			name:      "valid tmp dir path in text",
			input:     "The project's temporary directory is: /Users/ianshaw/.gemini/tmp/f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740\nOther text",
			wantMatch: true,
			wantHash:  "f7851b009ed314d1baee62e83115f486160283f4a55a582d89fdac8b9fe3b740",
		},
		{
			name:      "invalid hash length",
			input:     "/Users/ianshaw/.gemini/tmp/abc123",
			wantMatch: false,
		},
		{
			name:      "no tmp dir",
			input:     "Hello world",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match := GeminiCLITmpDirRegex.FindStringSubmatch(tt.input)
			if tt.wantMatch {
				require.NotNil(t, match, "expected regex to match")
				require.Len(t, match, 2, "expected 2 capture groups")
				require.Equal(t, tt.wantHash, match[1], "hash mismatch")
			} else {
				require.Nil(t, match, "expected regex not to match")
			}
		})
	}
}

func TestSafeShortPrefix(t *testing.T) {
	tests := []struct {
		name  string
		input string
		n     int
		want  string
	}{
		{name: "空字符串", input: "", n: 8, want: ""},
		{name: "长度小于截断值", input: "abc", n: 8, want: "abc"},
		{name: "长度等于截断值", input: "12345678", n: 8, want: "12345678"},
		{name: "长度大于截断值", input: "1234567890", n: 8, want: "12345678"},
		{name: "截断值为0", input: "123456", n: 0, want: "123456"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, GeminiShortPrefix(tt.input, tt.n))
		})
	}
}

func TestGeminiNativePrefacePreservesModerationBeforeMapping(t *testing.T) {
	base := &prefaceBackend{key: prefaceKey(), block: true}
	p := &geminiPrefaceBackend{base: base}
	h := NewGeminiNativeHandler(GeminiNativeOptions{}, p, base, nil, nil, p)
	c, w := prefaceContext(`not-json`)
	c.Params = gin.Params{{Key: "modelAction", Value: "/m:generateContent"}}
	h.GeminiV1BetaModels(c)
	require.Equal(t, []string{"request:m", "endpoint", "prompt:gemini", "moderate:gemini"}, base.events)
	require.Contains(t, w.Body.String(), "blocked")
}

func TestGeminiNativePrefaceCarriesSignatureAndDigestState(t *testing.T) {
	for _, bound := range []int64{65, 0} {
		t.Run(map[bool]string{true: "sticky", false: "digest"}[bound > 0], func(t *testing.T) {
			base := &prefaceBackend{key: prefaceKey()}
			p := &geminiPrefaceBackend{base: base, bound: bound}
			h := NewGeminiNativeHandler(GeminiNativeOptions{}, p, base, prefaceConcurrency(), func() string { t.Fatal("matched session must not create id"); return "" }, p)
			c, w := prefaceContext(`{"contents":[{"role":"user","parts":[{"text":"/.gemini/tmp/` + strings.Repeat("a", 64) + `"}]}]}`)
			c.Params = gin.Params{{Key: "modelAction", Value: "/original:streamGenerateContent"}}
			h.GeminiV1BetaModels(c)
			require.Equal(t, 200, w.Code, w.Body.String())
			require.Equal(t, "original", p.call.Model)
			require.Equal(t, "mapped-model", p.call.ModelName)
			require.True(t, p.call.Stream)
			require.True(t, p.call.HasBoundSession)
			want := bound
			if want == 0 {
				want = 87
				require.Equal(t, "previous", p.call.MatchedDigestChain)
				require.Equal(t, "session-id", p.call.SessionUUID)
				require.Contains(t, base.events, "sticky")
			}
			require.Equal(t, want, p.call.SignatureState.BoundProviderID)
			require.Equal(t, want, p.call.BoundProviderID)
			require.Equal(t, bound == 0, p.call.UseDigestFallback)
		})
	}
}
