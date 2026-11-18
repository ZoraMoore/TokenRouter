import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { getAPIActivity } from '@/api/admin/users'
import UserAPIActivity from '../UserAPIActivity.vue'

vi.mock('@/api/admin/users', () => ({ getAPIActivity: vi.fn() }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

afterEach(() => { vi.useRealTimers(); vi.clearAllMocks(); vi.restoreAllMocks() })

describe('用户管理活跃人数', () => {
  it('展示全站去重人数，定时更新并在卸载后停止请求', async () => {
    vi.useFakeTimers()
    vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('visible')
    vi.mocked(getAPIActivity).mockResolvedValue({ active_users: 12, window_seconds: 300, complete: true, reporting_nodes: 3, total_nodes: 3, generated_at: '' })
    const wrapper = mount(UserAPIActivity)
    await flushPromises()
    expect(wrapper.text()).toContain('12')
    vi.mocked(getAPIActivity).mockResolvedValue({ active_users: 15, window_seconds: 300, complete: true, reporting_nodes: 3, total_nodes: 3, generated_at: '' })
    await vi.advanceTimersByTimeAsync(15000)
    expect(wrapper.text()).toContain('15')
    wrapper.unmount()
    const calls = vi.mocked(getAPIActivity).mock.calls.length
    await vi.advanceTimersByTimeAsync(15000)
    expect(getAPIActivity).toHaveBeenCalledTimes(calls)
  })

  it('统计失败时展示不可用，避免误显示为零人', async () => {
    vi.mocked(getAPIActivity).mockRejectedValue(new Error('offline'))
    const wrapper = mount(UserAPIActivity)
    await flushPromises()
    expect(wrapper.text()).toContain('admin.users.activityUnavailable')
    wrapper.unmount()
  })
})
