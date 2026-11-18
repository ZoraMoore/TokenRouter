package egress

import (
	"strings"
	"time"
)

// TLSFingerprintProfile TLS 指纹配置模板
// 包含完整的 ClientHello 参数，用于模拟特定客户端的 TLS 握手特征。
type TLSFingerprintProfile struct {
	ID                      int64     `json:"id"`
	Name                    string    `json:"name"`
	Description             *string   `json:"description"`
	EnableGREASE            bool      `json:"enable_grease"`
	CipherSuites            []uint16  `json:"cipher_suites"`
	Curves                  []uint16  `json:"curves"`
	PointFormats            []uint16  `json:"point_formats"`
	SignatureAlgorithms     []uint16  `json:"signature_algorithms"`
	SignatureAlgorithmsCert []uint16  `json:"signature_algorithms_cert"`
	ALPNProtocols           []string  `json:"alpn_protocols"`
	SupportedVersions       []uint16  `json:"supported_versions"`
	KeyShareGroups          []uint16  `json:"key_share_groups"`
	PSKModes                []uint16  `json:"psk_modes"`
	Extensions              []uint16  `json:"extensions"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// Validate 检查模板名称。
func (p *TLSFingerprintProfile) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return &ValidationError{Field: "name", Message: "name is required"}
	}
	return nil
}
