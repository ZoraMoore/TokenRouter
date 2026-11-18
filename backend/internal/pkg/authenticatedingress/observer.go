package authenticatedingress

type AuthenticatedIngressRequestSnapshot struct {
	UserAgent          string
	Originator         string
	EdgeToken          string
	ConnectionID       string
	ConnectionRequests string
	RemoteAddress      string
	ResolvedAddress    string
	RemotePort         string
	ServerAddress      string
	ServerPort         string
	SessionReused      string
	ALPN               string
	HTTPProtocol       string
}

// AuthenticatedIngressIdentity 只证明请求已通过鉴权，并提供不可逆 owner 配额所需的内部数字 ID。
type AuthenticatedIngressIdentity struct {
	APIKeyID int64
	UserID   int64
}

// AuthenticatedIngressRequestObserver 接收已经通过 API Key 鉴权的白名单入站快照。
// 实现只能获得两个内部数字 ID，不会接触完整 APIKey 对象或原始凭据。
type AuthenticatedIngressRequestObserver interface {
	ObserveAuthenticatedIngressRequest(request AuthenticatedIngressRequestSnapshot, identity AuthenticatedIngressIdentity)
}
