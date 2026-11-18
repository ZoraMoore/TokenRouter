package tlscapture

type CapturedTLSExtension struct {
	Type   uint16 `json:"type"`
	Length uint16 `json:"length"`
}

type CapturedKeyShare struct {
	Group             uint16 `json:"group"`
	KeyExchangeLength uint16 `json:"key_exchange_length"`
}

type CapturedClientHello struct {
	RecordVersion               uint16                 `json:"record_version"`
	RecordVersions              []uint16               `json:"record_versions"`
	RecordLengths               []uint16               `json:"record_lengths"`
	HandshakeVersion            uint16                 `json:"handshake_version"`
	LegacyVersion               uint16                 `json:"legacy_version"`
	HandshakeLength             uint32                 `json:"handshake_length"`
	SessionIDLength             uint8                  `json:"session_id_length"`
	CompressionMethods          []uint8                `json:"compression_methods"`
	ExtensionsLength            uint16                 `json:"extensions_length"`
	ExtensionMetadata           []CapturedTLSExtension `json:"extension_metadata"`
	CipherSuites                []uint16               `json:"cipher_suites"`
	Curves                      []uint16               `json:"curves"`
	PointFormats                []uint16               `json:"point_formats"`
	SignatureAlgorithms         []uint16               `json:"signature_algorithms"`
	SignatureAlgorithmsCert     []uint16               `json:"signature_algorithms_cert"`
	ALPNProtocols               []string               `json:"alpn_protocols"`
	SupportedVersions           []uint16               `json:"supported_versions"`
	KeyShareGroups              []uint16               `json:"key_share_groups"`
	KeyShares                   []CapturedKeyShare     `json:"key_shares"`
	PSKModes                    []uint16               `json:"psk_modes"`
	Extensions                  []uint16               `json:"extensions"`
	PaddingLength               uint16                 `json:"padding_length"`
	SessionTicketLength         uint16                 `json:"session_ticket_length"`
	HasPreSharedKey             bool                   `json:"has_pre_shared_key"`
	PreSharedKeyExtensionLength uint16                 `json:"pre_shared_key_extension_length"`
	EnableGREASE                bool                   `json:"enable_grease"`
	JA3Raw                      string                 `json:"ja3_raw"`
	JA3Hash                     string                 `json:"ja3_hash"`
}

type ProfileValidationFinding struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ProfileValidationReport struct {
	Reusable        bool                       `json:"reusable"`
	ExactJA3        bool                       `json:"exact_ja3"`
	ExactStructure  bool                       `json:"exact_structure"`
	CapturedJA3Hash string                     `json:"captured_ja3_hash"`
	ReplayedJA3Hash string                     `json:"replayed_ja3_hash"`
	Issues          []ProfileValidationFinding `json:"issues"`
	Warnings        []ProfileValidationFinding `json:"warnings"`
}
