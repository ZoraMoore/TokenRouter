package payment

import (
	"github.com/shopspring/decimal"
)

// FeeConfig 表示某个支付渠道实际生效的手续费配置。
type FeeConfig struct {
	FixedFee float64 `json:"fixed_fee"`
	FeeRate  float64 `json:"fee_rate"`
}

// FeeBreakdown 表示一次下单的手续费拆分结果。
type FeeBreakdown struct {
	BaseAmount    float64 `json:"base_amount"`
	FixedFee      float64 `json:"fee_fixed"`
	FeeRate       float64 `json:"fee_rate"`
	FeeRateAmount float64 `json:"fee_rate_amount"`
	FeeAmount     float64 `json:"fee_amount"`
	PayAmount     float64 `json:"pay_amount"`
}

// CalculatePayAmountForCurrency 按币种精度计算比例手续费后的应付金额。
func CalculatePayAmountForCurrency(rechargeAmount float64, feeRate float64, currency string) string {
	return CalculatePayAmountWithFeeForCurrency(rechargeAmount, FeeConfig{FeeRate: feeRate}, currency).PayAmountStringForCurrency(currency)
}

// CalculatePayAmountWithFeeForCurrency 按币种精度计算固定手续费和比例手续费拆分后的实付金额。
func CalculatePayAmountWithFeeForCurrency(baseAmount float64, cfg FeeConfig, currency string) FeeBreakdown {
	fractionDigits := int32(CurrencyMaxFractionDigits(currency))
	amount := decimal.NewFromFloat(baseAmount).Round(fractionDigits)
	fixedFee := decimal.Zero
	if cfg.FixedFee > 0 {
		fixedFee = decimal.NewFromFloat(cfg.FixedFee).Round(fractionDigits)
	}

	rateAmount := decimal.Zero
	if cfg.FeeRate > 0 {
		rate := decimal.NewFromFloat(cfg.FeeRate)
		rateAmount = amount.Mul(rate).Div(decimal.NewFromInt(100)).RoundUp(fractionDigits)
	}

	feeAmount := fixedFee.Add(rateAmount).Round(fractionDigits)
	payAmount := amount.Add(feeAmount).Round(fractionDigits)
	return FeeBreakdown{
		BaseAmount:    decimalToFloat(amount),
		FixedFee:      decimalToFloat(fixedFee),
		FeeRate:       cfg.FeeRate,
		FeeRateAmount: decimalToFloat(rateAmount),
		FeeAmount:     decimalToFloat(feeAmount),
		PayAmount:     decimalToFloat(payAmount),
	}
}

// PayAmountStringForCurrency 按币种精度返回支付网关需要的字符串金额。
func (b FeeBreakdown) PayAmountStringForCurrency(currency string) string {
	return decimal.NewFromFloat(b.PayAmount).StringFixed(int32(CurrencyMaxFractionDigits(currency)))
}

func decimalToFloat(v decimal.Decimal) float64 {
	out, _ := v.Float64()
	return out
}
