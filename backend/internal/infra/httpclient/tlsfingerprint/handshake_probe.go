package tlsfingerprint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	utls "github.com/refraction-networking/utls" //nolint:revive // uTLS 构造器统一使用 utls 标识。
)

// TLSHandshakeResult 只包含 TLS 协商结果，不包含证书、目标响应或应用层数据。
type TLSHandshakeResult struct {
	Version     uint16 `json:"version"`
	CipherSuite uint16 `json:"cipher_suite"`
	ALPN        string `json:"alpn"`
}

// ProbeTLSHandshake 对已由上层完成白名单校验的 host:port 只执行一次 TLS 握手。
// 成功后立即关闭连接，既不发送 HTTP 请求，也不读取应用层响应。
func ProbeTLSHandshake(ctx context.Context, profile *Profile, target string) (*TLSHandshakeResult, error) {
	if ctx == nil {
		return nil, errors.New("TLS handshake probe requires a context")
	}
	host, port, err := net.SplitHostPort(strings.TrimSpace(target))
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return nil, fmt.Errorf("invalid TLS handshake target %q: expected host:port", target)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return nil, fmt.Errorf("invalid TLS handshake target port %q", port)
	}
	if err := ValidateProfile(profile); err != nil {
		return nil, fmt.Errorf("validate TLS fingerprint profile: %w", err)
	}

	conn, err := NewDialer(profile, nil).DialTLSContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	stateConn, ok := conn.(interface {
		ConnectionState() utls.ConnectionState
	})
	if !ok {
		return nil, errors.New("TLS handshake result is unavailable")
	}
	state := stateConn.ConnectionState()
	return &TLSHandshakeResult{
		Version:     state.Version,
		CipherSuite: state.CipherSuite,
		ALPN:        strings.TrimSpace(state.NegotiatedProtocol),
	}, nil
}
