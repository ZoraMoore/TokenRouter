import { formatPaymentAmount, paymentCurrencyFractionDigits } from './currency'
import type { RechargeCouponTerms } from '@/types/payment'

export interface RechargeCouponInput {
  currency: string
  minimum: string
  discount: string
}

export function emptyRechargeCouponInput(): RechargeCouponInput {
  return { currency: 'CNY', minimum: '', discount: '' }
}

// 按字符串转成整数，超出币种小数位的金额由表单提示修改。
export function parseRechargeCouponTerms(input: RechargeCouponInput): RechargeCouponTerms | null {
  const currency = input.currency.trim().toUpperCase()
  if (!/^[A-Z]{3}$/.test(currency)) return null
  const digits = paymentCurrencyFractionDigits(currency)
  const minimum = readCouponMinor(input.minimum, digits)
  const discount = readCouponMinor(input.discount, digits)
  if (minimum === null || discount === null || discount >= minimum) return null
  return { currency, minimum_minor: minimum, discount_minor: discount }
}

function readCouponMinor(value: string, digits: number): number | null {
  const parts = /^([0-9]{1,16})(?:\.([0-9]{1,3}))?$/.exec(value.trim())
  if (!parts) return null
  const fraction = parts[2] || ''
  if (fraction.length > digits) return null
  const amount = Number(parts[1] + fraction.padEnd(digits, '0'))
  return Number.isSafeInteger(amount) && amount > 0 ? amount : null
}

export function rechargeCouponAmount(minor: number, currency: string): number {
  return minor / (10 ** paymentCurrencyFractionDigits(currency))
}

export function formatRechargeCouponAmount(minor: number, currency: string): string {
  return formatPaymentAmount(rechargeCouponAmount(minor, currency), currency)
}

export function rechargeCouponLabel(terms: RechargeCouponTerms) {
  return {
    minimum: formatRechargeCouponAmount(terms.minimum_minor, terms.currency),
    discount: formatRechargeCouponAmount(terms.discount_minor, terms.currency),
    currency: terms.currency,
  }
}
