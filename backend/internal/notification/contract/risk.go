package contract

import "time"

type (
	RiskPolicy struct{ BanThreshold, CyberBanThreshold int }
	RiskLog    struct {
		ID                                    int64
		UserID                                *int64
		UserEmail, GroupName, HighestCategory string
		HighestScore                          float64
		ViolationCount                        int
		AutoBanned                            bool
		CreatedAt                             time.Time
	}
)

type RiskWarning struct {
	ID                                 int64
	UserID                             *int64
	UserEmail, GroupName, ProviderName string
	ViolationCount                     int
	CreatedAt                          time.Time
}
