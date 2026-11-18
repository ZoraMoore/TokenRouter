import { beforeEach, describe, expect, it, vi } from 'vitest'

const { get, post } = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn()
}))

vi.mock('@/api/client', () => ({
  apiClient: { get, post }
}))

import { createCollectorSession, list } from '@/api/admin/tlsFingerprintProfile'

describe('管理员 TLS 指纹模板 API', () => {
  beforeEach(() => {
    get.mockReset()
    post.mockReset()
  })

  it('读取模板列表时不带查询参数', async () => {
    const profiles = [{ id: 9, name: '手动配置的模板' }]
    get.mockResolvedValue({ data: profiles })

    await expect(list()).resolves.toEqual(profiles)
    expect(get).toHaveBeenCalledWith('/admin/tls-fingerprint-profiles')
  })

  it('创建诊断采集会话时发送空请求体', async () => {
    const session = {
      token: 'test-token',
      expires_at: '2026-06-01T00:30:00Z',
      capture_url: 'https://collector.example:8443/capture/test-token'
    }
    post.mockResolvedValue({ data: session })

    await expect(createCollectorSession()).resolves.toEqual(session)
    expect(post).toHaveBeenCalledWith('/admin/tls-fingerprint-profiles/collector/sessions')
  })
})
