import { describe, expect, it } from 'vitest'
import { billingSettingsToAPI, defaultBillingSettings, validateBillingSettings } from '../billingSettings'

describe('shared billing settings', () => {
  it('keeps defaults, explicit zero and cleared prices distinct', () => {
    const settings = defaultBillingSettings()
    expect(settings.long_context_pricing_enabled).toBe(true)
    expect(settings.image_rate_independent).toBe(false)
    expect(settings.image_rate_multiplier).toBe(1)
    settings.image_rate_independent = true
    settings.image_rate_multiplier = 0
    expect(billingSettingsToAPI(settings)).toMatchObject({ image_rate_independent: true, image_rate_multiplier: 0 })
    expect(settings.batch_image_discount_multiplier).toBe(0.5)
    expect(settings.batch_image_hold_multiplier).toBe(0.6)
    settings.search_price_per_1k = 0
    settings.web_search_price_per_call = '' as unknown as number
    expect(billingSettingsToAPI(settings)).toMatchObject({ search_price_per_1k: 0, web_search_price_per_call: null })
  })

  it('validates the merged hold ratio and same-day peak window', () => {
    const settings = defaultBillingSettings()
    settings.batch_image_discount_multiplier = 0.7
    expect(validateBillingSettings(settings)).toBe('invalidHold')
    settings.batch_image_hold_multiplier = 0.8
    settings.peak_rate_enabled = true
    settings.peak_start = '22:00'
    settings.peak_end = '02:00'
    expect(validateBillingSettings(settings)).toBe('invalidPeak')
    settings.peak_end = '23:00'
    settings.peak_rate_multiplier = 0
    expect(validateBillingSettings(settings)).toBeNull()
    settings.audio_stt_price_per_hour = -1
    expect(validateBillingSettings(settings)).toBe('invalidNumber')
  })
})
