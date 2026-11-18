import { beforeEach, describe, expect, it, vi } from 'vitest'
import { readStorageWithLegacyKey } from '../storage'
import { hasAcceptedLoginAgreement, revokeLoginAgreement } from '../loginAgreement'

describe('品牌存储键兼容', () => {
  beforeEach(() => localStorage.clear())

  it('继承旧值，保留原键，并优先使用新键', () => {
    localStorage.setItem('sub2api_locale', 'zh')
    expect(readStorageWithLegacyKey(localStorage, 'tokenrouter_locale', 'sub2api_locale')).toBe('zh')
    expect(localStorage.getItem('tokenrouter_locale')).toBe('zh')
    expect(localStorage.getItem('sub2api_locale')).toBe('zh')
    localStorage.setItem('tokenrouter_locale', 'en')
    expect(readStorageWithLegacyKey(localStorage, 'tokenrouter_locale', 'sub2api_locale')).toBe('en')
  })

  it('旧协议确认仍校验 revision，新键的撤回记录阻止回退', () => {
    localStorage.setItem('sub2api_login_agreement_consent', JSON.stringify({ revision: 'v1' }))
    expect(hasAcceptedLoginAgreement('v1')).toBe(true)
    expect(hasAcceptedLoginAgreement('v2')).toBe(false)
    revokeLoginAgreement()
    expect(hasAcceptedLoginAgreement('v1')).toBe(false)
  })

  it('复制失败仍返回旧值，读取被禁止时安全降级', () => {
    const storage = { getItem: vi.fn((key: string) => key === 'old' ? 'zh' : null), setItem: vi.fn(() => { throw new Error('quota') }) } as unknown as Storage
    expect(readStorageWithLegacyKey(storage, 'new', 'old')).toBe('zh')
    vi.mocked(storage.getItem).mockImplementation(() => { throw new Error('blocked') })
    expect(readStorageWithLegacyKey(storage, 'new', 'old')).toBeNull()
  })
})
