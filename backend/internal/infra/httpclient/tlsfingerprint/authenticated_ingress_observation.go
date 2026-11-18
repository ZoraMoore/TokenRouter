package tlsfingerprint

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
)

const (
	// AuthenticatedIngressObservationSchemaVersion 是 observer 与主服务之间的载荷版本。
	AuthenticatedIngressObservationSchemaVersion = 1
	authenticatedIngressFlowDomain               = "tokenrouter/authenticated-ingress-flow/v1"
)

// AuthenticatedIngressObservation 是主机 observer 提交给主服务的安全采集结果。
// FlowID 是五元组的带密钥摘要；载荷不包含 IP、端口、原始包或 TLS 密钥材料。
type AuthenticatedIngressObservation struct {
	SchemaVersion int                  `json:"schema_version"`
	FlowID        string               `json:"flow_id"`
	ObservedAt    time.Time            `json:"observed_at"`
	ServerName    string               `json:"server_name"`
	ClientHello   *CapturedClientHello `json:"client_hello"`
}

// BuildAuthenticatedIngressFlowID 为同一入站 TCP 五元组生成稳定、不可逆的关联 ID。
// 地址会先规范化，确保 Nginx 与 observer 对 IPv4-mapped IPv6 使用相同输入。
func BuildAuthenticatedIngressFlowID(
	secret string,
	sourceAddress string,
	sourcePort uint16,
	destinationAddress string,
	destinationPort uint16,
) (string, error) {
	if len(secret) < 32 {
		return "", errors.New("authenticated ingress observer token must be at least 32 bytes")
	}
	source, err := parseAuthenticatedIngressAddress(sourceAddress)
	if err != nil {
		return "", fmt.Errorf("invalid source address: %w", err)
	}
	destination, err := parseAuthenticatedIngressAddress(destinationAddress)
	if err != nil {
		return "", fmt.Errorf("invalid destination address: %w", err)
	}
	if sourcePort == 0 || destinationPort == 0 {
		return "", errors.New("authenticated ingress flow ports must be non-zero")
	}

	// 使用长度前缀避免 IPv6 冒号或字段拼接产生歧义；不把明文输入放入结果。
	canonical := fmt.Sprintf(
		"%s\x00tcp\x00%d:%s\x00%d\x00%d:%s\x00%d",
		authenticatedIngressFlowDomain,
		len(source.String()), source.String(), sourcePort,
		len(destination.String()), destination.String(), destinationPort,
	)
	key := sha256.Sum256([]byte("flow-id\x00" + secret))
	mac := hmac.New(sha256.New, key[:])
	_, _ = mac.Write([]byte(canonical))
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func parseAuthenticatedIngressAddress(value string) (netip.Addr, error) {
	value = strings.TrimSpace(value)
	// Nginx 变量可能为 IPv6 地址保留方括号，关联前统一移除。
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	address, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Addr{}, err
	}
	return address.Unmap(), nil
}
