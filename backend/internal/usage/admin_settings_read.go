package usage

// AdminReadSettings 包含用量排行和用户错误记录的展示设置。
type AdminReadSettings struct {
	AllowUserViewErrorRequests  bool
	UsageRankingEnabled         bool
	UsageRankingLimit           int
	UsageRankingShowActualCost  bool
	UsageRankingShowRequests    bool
	UsageRankingShowTotalTokens bool
	UsageRankingSortBy          string
}

// ReadAdminSettings 从传入的设置值解析用量排行和用户错误记录的展示设置。
func ReadAdminSettings(settings map[string]string) *AdminReadSettings {
	usageRanking := ParseRankingSettings(settings)
	result := &AdminReadSettings{}

	result.UsageRankingLimit = usageRanking.Limit
	result.UsageRankingEnabled = usageRanking.Enabled
	result.UsageRankingSortBy = string(usageRanking.SortBy)
	result.UsageRankingShowTotalTokens = usageRanking.ShowTotalTokens
	result.UsageRankingShowRequests = usageRanking.ShowRequests
	result.UsageRankingShowActualCost = usageRanking.ShowActualCost
	result.AllowUserViewErrorRequests = settings[SettingKeyAllowUserViewErrorRequests] == "true"
	return result
}
