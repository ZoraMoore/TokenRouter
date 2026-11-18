package authenticatedingress

type Config struct {
	Enabled                       bool     `mapstructure:"enabled"`                                   // 是否启用已认证入站关联
	EdgeToken                     string   `mapstructure:"edge_token"`                                // Nginx 覆盖注入头的共享 token
	ObserverToken                 string   `mapstructure:"observer_token"`                            // 观察器提交与 flow HMAC token
	ObservedHosts                 []string `mapstructure:"observed_hosts"`                            // 允许作为入站来源的 SNI
	ValidationTargets             []string `mapstructure:"validation_targets"`                        // 自动执行 TLS-only 验证的目标
	PendingTTLSeconds             int      `mapstructure:"pending_ttl_seconds"`                       // 未匹配抓包/鉴权事件 TTL
	CandidateTTLSeconds           int      `mapstructure:"candidate_ttl_seconds"`                     // 未达到样本数的候选 TTL
	MinimumIndependentConnections int      `mapstructure:"minimum_independent_connections"`           // 自动验证所需独立完整握手数
	MaxPendingFlows               int      `mapstructure:"max_pending_flows"`                         // 内存 pending flow 上限
	MaxCandidateGroups            int      `mapstructure:"max_candidate_groups"`                      // 内存候选组上限
	MaxValidationAttemptsPerDay   int      `mapstructure:"max_validation_attempts_per_day"`           // 每日自动验证尝试上限
	MaxValidationAttemptsPerOwner int      `mapstructure:"max_validation_attempts_per_owner_per_day"` // 单个 owner 每日验证上限
	MaxVerifiedProfilesPerDay     int      `mapstructure:"max_verified_profiles_per_day"`             // 每日自动创建模板上限
	AutoCreateVerified            bool     `mapstructure:"auto_create_verified"`                      // 验证成功后是否自动创建模板
}
