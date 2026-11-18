import { readStorageWithLegacyKey } from './storage'

export const LOGIN_AGREEMENT_STORAGE_KEY = 'tokenrouter_login_agreement_consent'

// 撤回时在新键中记录拒绝状态，下次读取以该记录覆盖旧品牌的同意记录。
export function revokeLoginAgreement(): void {
  if (typeof window === 'undefined') return
  window.localStorage.setItem(LOGIN_AGREEMENT_STORAGE_KEY, '{}')
}

// 登录协议按 revision 记录，条款更新后旧确认不会继续放行快捷登录。
export function hasAcceptedLoginAgreement(revision: string): boolean {
  if (!revision || typeof window === 'undefined') return false
  try {
    const raw = readStorageWithLegacyKey(window.localStorage, LOGIN_AGREEMENT_STORAGE_KEY, 'sub2api_login_agreement_consent')
    if (!raw) return false
    const parsed = JSON.parse(raw) as { revision?: string }
    return parsed.revision === revision
  } catch {
    return false
  }
}
