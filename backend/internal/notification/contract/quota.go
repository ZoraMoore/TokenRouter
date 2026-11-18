package contract

type QuotaDimension struct {
	Name               string
	Enabled            bool
	Threshold          float64
	ThresholdType      string
	CurrentUsed, Limit float64
}
