// 共享计费设置与具体模型价卡独立生效。
export interface BillingSettings {
  image_rate_independent: boolean
  image_rate_multiplier: number
  peak_rate_enabled: boolean
  peak_start: string
  peak_end: string
  peak_rate_multiplier: number
  long_context_pricing_enabled: boolean
  free_openai_fast: boolean
  batch_image_discount_multiplier: number
  batch_image_hold_multiplier: number
  web_search_price_per_call: number | null
  search_price_per_1k: number | null
  audio_realtime_price_per_min: number | null
  audio_tts_price_per_million_chars: number | null
  audio_stt_price_per_hour: number | null
}

export const defaultBillingSettings = (): BillingSettings => ({
  image_rate_independent: false,
  image_rate_multiplier: 1,
  peak_rate_enabled: false,
  peak_start: "",
  peak_end: "",
  peak_rate_multiplier: 1,
  long_context_pricing_enabled: true,
  free_openai_fast: false,
  batch_image_discount_multiplier: 0.5,
  batch_image_hold_multiplier: 0.6,
  web_search_price_per_call: null,
  search_price_per_1k: null,
  audio_realtime_price_per_min: null,
  audio_tts_price_per_million_chars: null,
  audio_stt_price_per_hour: null,
})

// 数字输入清空时归一为空值，零价保持原值。
export function billingSettingsToAPI(value: BillingSettings): BillingSettings {
  const result = { ...value }
  result.web_search_price_per_call = value.web_search_price_per_call === null || (value.web_search_price_per_call as unknown) === '' ? null : Number(value.web_search_price_per_call)
  result.search_price_per_1k = value.search_price_per_1k === null || (value.search_price_per_1k as unknown) === '' ? null : Number(value.search_price_per_1k)
  result.audio_realtime_price_per_min = value.audio_realtime_price_per_min === null || (value.audio_realtime_price_per_min as unknown) === '' ? null : Number(value.audio_realtime_price_per_min)
  result.audio_tts_price_per_million_chars = value.audio_tts_price_per_million_chars === null || (value.audio_tts_price_per_million_chars as unknown) === '' ? null : Number(value.audio_tts_price_per_million_chars)
  result.audio_stt_price_per_hour = value.audio_stt_price_per_hour === null || (value.audio_stt_price_per_hour as unknown) === '' ? null : Number(value.audio_stt_price_per_hour)
  return result
}

export function validateBillingSettings(value: BillingSettings): string | null {
 const s = billingSettingsToAPI(value)
 const rates = [s.image_rate_multiplier, s.peak_rate_multiplier, s.batch_image_discount_multiplier, s.batch_image_hold_multiplier]
 if (rates.some(v => typeof v !== 'number' || !Number.isFinite(v) || v < 0)) return 'invalidNumber'
 if (s.web_search_price_per_call !== null && (!Number.isFinite(s.web_search_price_per_call) || s.web_search_price_per_call < 0)) return 'invalidNumber'
 if (s.search_price_per_1k !== null && (!Number.isFinite(s.search_price_per_1k) || s.search_price_per_1k < 0)) return 'invalidNumber'
 if (s.audio_realtime_price_per_min !== null && (!Number.isFinite(s.audio_realtime_price_per_min) || s.audio_realtime_price_per_min < 0)) return 'invalidNumber'
 if (s.audio_tts_price_per_million_chars !== null && (!Number.isFinite(s.audio_tts_price_per_million_chars) || s.audio_tts_price_per_million_chars < 0)) return 'invalidNumber'
 if (s.audio_stt_price_per_hour !== null && (!Number.isFinite(s.audio_stt_price_per_hour) || s.audio_stt_price_per_hour < 0)) return 'invalidNumber'
 if (s.batch_image_hold_multiplier < s.batch_image_discount_multiplier) return 'invalidHold'
 if (s.peak_rate_enabled && (!/^([01]\d|2[0-3]):[0-5]\d$/.test(s.peak_start) || !/^([01]\d|2[0-3]):[0-5]\d$/.test(s.peak_end) || s.peak_start >= s.peak_end)) return 'invalidPeak'
 return null
}
