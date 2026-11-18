package provider

import (
	"github.com/TokenFlux/TokenRouter/internal/provider"
	"github.com/TokenFlux/TokenRouter/internal/upstream/openai"
)

// DecodeArchiveIDToken 解码 OpenAI 导入的身份提示，调用方负责身份验证。
func DecodeArchiveIDToken(token string) (*provider.ArchiveIdentityHints, error) {
	claims, err := openai.DecodeIDToken(token)
	if err != nil {
		return nil, err
	}
	info := claims.GetUserInfo()
	if info == nil {
		return nil, nil
	}
	return &provider.ArchiveIdentityHints{Email: info.Email, PlanType: info.PlanType, ChatGPTAccountID: info.ChatGPTAccountID, ChatGPTUserID: info.ChatGPTUserID, OrganizationID: info.OrganizationID}, nil
}
