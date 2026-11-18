package grok

// TokenResponse 表示 xAI OAuth token 响应。
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	IDToken      string `json:"id_token,omitempty"`
	TokenType    string `json:"token_type,omitempty"`
	ExpiresIn    int64  `json:"expires_in,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// PasswordLoginResult 表示临时的密码登录结果。
// SSOToken 是传给 ConvertSSOToBuild 的临时凭据，调用方需要在转换后丢弃。
type PasswordLoginResult struct {
	Email    string `json:"email,omitempty"`
	SSOToken string `json:"sso_token"`
}

// AuthorizationInput 是解析后的手动 OAuth 回调输入。
type AuthorizationInput struct {
	Code          string
	State         string
	RequiresState bool
}
