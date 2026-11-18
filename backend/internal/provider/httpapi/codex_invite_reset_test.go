package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/TokenFlux/TokenRouter/internal/provider"
)

// codexInviteCommandsStub 记录管理端输入，测试期间不会发起邀请或扣除次数。
type codexInviteCommandsStub struct {
	calls  int
	lastID int64
	invite provider.CodexInviteRequest
	credit string
}

func (s *codexInviteCommandsStub) GetStatus(_ context.Context, id int64) (*provider.CodexInviteResetStatus, error) {
	s.calls++
	s.lastID = id
	return &provider.CodexInviteResetStatus{}, nil
}

func (s *codexInviteCommandsStub) SendInvite(_ context.Context, id int64, input provider.CodexInviteRequest) (*provider.CodexInviteResetInviteResult, error) {
	s.calls++
	s.lastID, s.invite = id, input
	return &provider.CodexInviteResetInviteResult{SentCount: 1}, nil
}

func (s *codexInviteCommandsStub) Consume(_ context.Context, id int64, credit string) (*provider.CodexInviteResetConsumeResult, error) {
	s.calls++
	s.lastID, s.credit = id, credit
	return &provider.CodexInviteResetConsumeResult{}, nil
}

func TestCodexInviteResetHandlerValidatesAndForwardsAdminRequests(t *testing.T) {
	const confirmed = `{"emails":["friend@example.com"],"program_id":"codex_referral_consumer","confirmed":true}`
	for _, tc := range []struct {
		name, id, action, body string
		status                 int
	}{
		{"status", "42", "status", "", 200},
		{"positive", "0", "status", "", 400},
		{"negative", "-1", "send", confirmed, 400},
		{"confirmed", "42", "send", confirmed, 200},
		{"unknown", "42", "send", `{"emails":[],"unknown":true}`, 400},
		{"missing", "42", "send", `{}`, 400},
		{"oversize", "42", "send", strings.Repeat(" ", 8193) + confirmed, 400},
		{"overlongEmail", "42", "send", `{"emails":["` + strings.Repeat("a", 255) + `@example.com"],"program_id":"codex_referral_consumer"}`, 400},
		{"credit", "42", "consume", `{"credit_id":"credit-2"}`, 200},
		{"automatic", "42", "consume", `{}`, 200},
		{"null", "42", "consume", `null`, 400},
		{"nullCredit", "42", "consume", `{"credit_id":null}`, 400},
		{"blankCredit", "42", "consume", `{"credit_id":"  "}`, 400},
		{"unknownCredit", "42", "consume", `{"unused":"x"}`, 400},
		{"trailing", "42", "consume", `{}{}`, 400},
		{"longCredit", "42", "consume", `{"credit_id":"` + strings.Repeat("x", 257) + `"}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &codexInviteCommandsStub{}
			h := NewCodexInviteResetHandler(s)
			router := gin.New()
			router.GET("/:id/status", h.GetStatus)
			router.POST("/:id/send", h.SendInvite)
			router.POST("/:id/consume", h.Consume)
			method := http.MethodPost
			if tc.action == "status" {
				method = http.MethodGet
			}
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, httptest.NewRequest(method, "/"+tc.id+"/"+tc.action, strings.NewReader(tc.body)))
			require.Equal(t, tc.status, rec.Code)
			require.Contains(t, rec.Header().Get("Cache-Control"), "no-store")
			if tc.status != 200 {
				require.Zero(t, s.calls)
				return
			}
			require.EqualValues(t, 42, s.lastID)
			switch tc.name {
			case "confirmed":
				require.True(t, s.invite.Confirmed)
				require.Equal(t, "codex_referral_consumer", s.invite.ProgramID)
				require.Equal(t, []string{"friend@example.com"}, s.invite.Emails)
			case "credit":
				require.Equal(t, "credit-2", s.credit)
			}
		})
	}
}
