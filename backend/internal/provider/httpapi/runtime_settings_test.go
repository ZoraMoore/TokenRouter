package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/settings"
)

// whitelistSettingsStore 记录管理接口测试中的单键读写。
type whitelistSettingsStore struct {
	values        map[string]string
	err           error
	commitOnError bool
}

func (s *whitelistSettingsStore) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", settings.ErrSettingNotFound
}

func (s *whitelistSettingsStore) Set(_ context.Context, key, value string) error {
	if s.err == nil || s.commitOnError {
		s.values[key] = value
	}
	return s.err
}

func TestOpenAIOAuthModelWhitelistSettingsHTTP(t *testing.T) {
	const path = "/api/v1/admin/settings/openai-oauth-model-whitelist"
	store := &whitelistSettingsStore{values: map[string]string{}}
	router := gin.New()
	handler := NewRuntimeSettingsHandler(provider.NewRuntimeSettings(store, settings.ErrSettingNotFound))
	RegisterProviderSettingsRoutes(router.Group("/api/v1/admin/settings"), handler)
	request := func(method, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		result := httptest.NewRecorder()
		router.ServeHTTP(result, req)
		return result
	}

	initial := request(http.MethodGet, "")
	require.Equal(t, http.StatusOK, initial.Code)
	require.Contains(t, initial.Body.String(), `"configured":false`)
	require.Contains(t, initial.Body.String(), `"codex-auto-review"`)
	require.Empty(t, store.values)

	body := `{"models":[" gpt-my-model ","gpt-my-model","vendor/model:1"]}`
	saved := request(http.MethodPut, body)
	require.Equal(t, http.StatusOK, saved.Code)
	expected := `{"code":0,"message":"success","data":{"configured":true,"models":["gpt-my-model","vendor/model:1"]}}`
	require.JSONEq(t, expected, saved.Body.String())
	require.JSONEq(t, expected, request(http.MethodGet, "").Body.String())

	for _, input := range []string{
		`{}`, `{"models":[]}`, `{"models":null}`, `{"models":["gpt-*"]}`,
		`{"models":"gpt-my-model"}`, `{"models":[4]}`,
		`{"models":["ok"],"configured":false}`, `{"models":["ok"]}{}`,
		strings.Repeat(" ", provider.OpenAIOAuthModelWhitelistMaxBytes) + body,
	} {
		require.Equal(t, http.StatusBadRequest, request(http.MethodPut, input).Code, input)
	}
	store.err = errors.New("settings store unavailable")
	failed := request(http.MethodPut, `{"models":["uncommitted"]}`)
	require.Equal(t, http.StatusInternalServerError, failed.Code)
	require.NotContains(t, failed.Body.String(), store.err.Error())
	require.JSONEq(t, expected, request(http.MethodGet, "").Body.String())

	// 数据库提交后连接中断，读取时应能发现新的持久化结果。
	store.commitOnError = true
	require.Equal(t, http.StatusInternalServerError, request(http.MethodPut, `{"models":["committed"]}`).Code)
	require.JSONEq(t, `{"code":0,"message":"success","data":{"configured":true,"models":["committed"]}}`, request(http.MethodGet, "").Body.String())
}
