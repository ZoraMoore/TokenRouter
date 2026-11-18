import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post }
}))

import { authenticatedIngressStatus, setApproval } from '@/api/admin/tlsFingerprintProfile'

describe('管理员 TLS 指纹模板 API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('从专用端点读取已认证入站聚合状态', async () => {
    const status = {
      enabled: true,
      pending_client_hellos: 2,
      pending_authenticated_flows: 1,
      candidate_groups: 1,
      validations_in_flight: 0,
      matched_flows: 8,
      rejected_flows: 0,
      verified_profiles_created: 1,
      existing_profiles_matched: 7
    }
    get.mockResolvedValue({ data: status })

    await expect(authenticatedIngressStatus()).resolves.toEqual(status)
    expect(get).toHaveBeenCalledWith('/admin/tls-fingerprint-profiles/collector/authenticated-ingress/status')
  })

  it('通过专用审批端点发布已验证模板且默认不加入随机池', async () => {
    const profile = { id: 9, name: 'authenticated-codex' }
    post.mockResolvedValue({ data: profile })

    await expect(setApproval(9, { approved_for_use: true, random_pool_eligible: false })).resolves.toEqual(profile)
    expect(post).toHaveBeenCalledWith('/admin/tls-fingerprint-profiles/9/approval', {
      approved_for_use: true,
      random_pool_eligible: false
    })
  })
})
