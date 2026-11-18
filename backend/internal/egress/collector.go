package egress

import (
	"context"
	"time"

	"github.com/TokenFlux/TokenRouter/internal/pkg/tlscapture"
)

type TLSFingerprintCollectorSessionRequest struct {
	Mode               string `json:"mode"`
	ClientKind         string `json:"client_kind"`
	ClientVersion      string `json:"client_version"`
	Runtime            string `json:"runtime"`
	RuntimeVersion     string `json:"runtime_version"`
	OperatingSystem    string `json:"operating_system"`
	Architecture       string `json:"architecture"`
	TargetHost         string `json:"target_host"`
	ColdStartConfirmed bool   `json:"cold_start_confirmed"`
}

// TLSFingerprintCollectorStatus 表示收集器当前运行状态。
type TLSFingerprintCollectorStatus struct {
	Running              bool       `json:"running"`
	ListenAddress        string     `json:"listen_address"`
	PublicBaseURL        string     `json:"public_base_url"`
	UsingGeneratedCert   bool       `json:"using_generated_cert"`
	CAPEM                string     `json:"ca_pem,omitempty"`
	SessionTTLSeconds    int        `json:"session_ttl_seconds"`
	MaxRecordsPerSession int        `json:"max_records_per_session"`
	StartedAt            *time.Time `json:"started_at,omitempty"`
	LastError            string     `json:"last_error,omitempty"`
}

// TLSFingerprintCollectorSession 表示一次短期采集会话。
type TLSFingerprintCollectorSession struct {
	Token              string    `json:"token"`
	ExpiresAt          time.Time `json:"expires_at"`
	CaptureURL         string    `json:"capture_url"`
	ProxyURL           string    `json:"proxy_url,omitempty"`
	Mode               string    `json:"mode"`
	ClientKind         string    `json:"client_kind,omitempty"`
	ClientVersion      string    `json:"client_version,omitempty"`
	Runtime            string    `json:"runtime,omitempty"`
	RuntimeVersion     string    `json:"runtime_version,omitempty"`
	OperatingSystem    string    `json:"operating_system,omitempty"`
	Architecture       string    `json:"architecture,omitempty"`
	TargetHost         string    `json:"target_host,omitempty"`
	ColdStartConfirmed bool      `json:"cold_start_confirmed"`
	MinimumColdSamples int       `json:"minimum_cold_samples"`
	CAPEM              string    `json:"ca_pem,omitempty"`
}

// TLSFingerprintCaptureRecord 表示一次采集结果。
type TLSFingerprintCaptureRecord struct {
	ID                  string                              `json:"id"`
	CapturedAt          time.Time                           `json:"captured_at"`
	ClientKind          string                              `json:"client_kind"`
	DetectedClientKind  string                              `json:"detected_client_kind,omitempty"`
	SourceKind          string                              `json:"source_kind"`
	CaptureMode         string                              `json:"capture_mode"`
	TargetHost          string                              `json:"target_host,omitempty"`
	PromotionEligible   bool                                `json:"promotion_eligible"`
	MatchingSampleCount int                                 `json:"matching_sample_count"`
	BlockedReasons      []string                            `json:"blocked_reasons"`
	ConnectionID        string                              `json:"-"`
	RequestPath         string                              `json:"request_path"`
	Method              string                              `json:"method"`
	UserAgent           string                              `json:"user_agent"`
	JA3Raw              string                              `json:"ja3_raw"`
	JA3Hash             string                              `json:"ja3_hash"`
	NegotiatedALPN      string                              `json:"negotiated_alpn"`
	HTTPProto           string                              `json:"http_proto"`
	Profile             *TLSFingerprintProfile              `json:"profile"`
	YAML                string                              `json:"yaml"`
	HeadersSummary      map[string]string                   `json:"headers_summary"`
	StainlessSummary    map[string]string                   `json:"stainless_summary"`
	RawTLSFingerprint   *CapturedClientHello                `json:"raw_tls_fingerprint,omitempty"`
	ReplayValidation    *tlscapture.ProfileValidationReport `json:"replay_validation,omitempty"`
}

type CapturedClientHello = tlscapture.CapturedClientHello

// Collector 是 HTTP 管理面消费的按需采集能力；监听由技术 Adapter 拥有。
type Collector interface {
	Status() TLSFingerprintCollectorStatus
	Start(context.Context) (TLSFingerprintCollectorStatus, error)
	Stop(context.Context) error
	Shutdown(context.Context) error
	CreateSession() (*TLSFingerprintCollectorSession, error)
	ListCaptures(string) ([]*TLSFingerprintCaptureRecord, error)
	DeleteSession(string)
}
