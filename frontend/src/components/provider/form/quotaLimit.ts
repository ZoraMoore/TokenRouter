import { reactive, type Ref } from 'vue'
import type { QuotaResetMode } from '@/constants/provider'

export interface QuotaLimitValues {
  totalLimit: number | null
  dailyLimit: number | null
  weeklyLimit: number | null
  dailyResetMode: QuotaResetMode | null
  dailyResetHour: number | null
  weeklyResetMode: QuotaResetMode | null
  weeklyResetDay: number | null
  weeklyResetHour: number | null
  resetTimezone: string | null
}

type QuotaLimitRefs = { [K in keyof QuotaLimitValues]: Ref<QuotaLimitValues[K]> }

// bindQuotaLimits 把弹窗中分散的额度草稿合成一个可读写视图，写入会直接回到原来的 ref。
export function bindQuotaLimits(refs: QuotaLimitRefs) {
  const limits = reactive(refs) as unknown as QuotaLimitValues
  function setLimit(key: keyof QuotaLimitValues, value: QuotaLimitValues[keyof QuotaLimitValues]) {
    (limits as Record<keyof QuotaLimitValues, unknown>)[key] = value
  }
  return { limits, setLimit }
}
