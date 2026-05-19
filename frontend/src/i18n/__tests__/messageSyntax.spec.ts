import { describe, expect, it } from 'vitest'

import en from '../locales/en'
import zh from '../locales/zh'

function collectUnescapedAtMessages(
  value: unknown,
  path: string,
  violations: string[],
) {
  if (typeof value === 'string') {
    if (/@(?!'\})/.test(value)) {
      violations.push(path)
    }
    return
  }

  if (!value || typeof value !== 'object') {
    return
  }

  for (const [key, child] of Object.entries(value)) {
    collectUnescapedAtMessages(child, `${path}.${key}`, violations)
  }
}

describe('i18n message syntax', () => {
  it('does not contain raw at signs that vue-i18n parses as linked messages', () => {
    const violations: string[] = []

    collectUnescapedAtMessages(en, 'en', violations)
    collectUnescapedAtMessages(zh, 'zh', violations)

    expect(violations).toEqual([])
  })

  it('escapes payment access email placeholders', () => {
    expect(en.admin.settings.payment.allowedEmailsPlaceholder).toBe(
      "dicardoteam{'@'}gmail.com",
    )
    expect(zh.admin.settings.payment.allowedEmailsPlaceholder).toBe(
      "dicardoteam{'@'}gmail.com",
    )
  })
})
