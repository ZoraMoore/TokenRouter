import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import UserDashboardLiveStats from '../UserDashboardLiveStats.vue'
import { usageAPI } from '@/api/usage'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
      locale: { value: 'en' },
    }),
  }
})

vi.mock('@/composables/useBalanceDisplay', () => ({
  useBalanceDisplay: () => ({
    formatBalanceAmount: (value: number) => `$${value}`,
  }),
}))

vi.mock('@/api/usage', () => ({
  usageAPI: {
    getDashboardStats: vi.fn(),
  },
}))

const stats = { rpm: 12, tpm: 3400, average_duration_ms: 850, today_actual_cost: 2.5 }

// setVisibility 模拟页面切到后台或回到前台。
const setVisibility = (state: DocumentVisibilityState) => {
  Object.defineProperty(document, 'visibilityState', { configurable: true, get: () => state })
  document.dispatchEvent(new Event('visibilitychange'))
}

describe('UserDashboardLiveStats', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    vi.mocked(usageAPI.getDashboardStats).mockReset()
    vi.mocked(usageAPI.getDashboardStats).mockResolvedValue(stats as any)
  })

  afterEach(() => {
    setVisibility('visible')
    vi.useRealTimers()
  })

  it('显示 RPM、TPM、平均耗时和今日消费', async () => {
    const wrapper = mount(UserDashboardLiveStats)
    await flushPromises()
    expect(wrapper.get('[data-testid="live-stat-rpm"]').text()).toContain('12')
    expect(wrapper.get('[data-testid="live-stat-latency"]').text()).toContain('850ms')
    expect(wrapper.get('[data-testid="live-stat-todayCost"]').text()).toContain('$2.5')
    wrapper.unmount()
  })

  it('每分钟轮询一次，页面隐藏时暂停，回到页面立即刷新', async () => {
    const wrapper = mount(UserDashboardLiveStats)
    await flushPromises()
    expect(usageAPI.getDashboardStats).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(60 * 1000)
    expect(usageAPI.getDashboardStats).toHaveBeenCalledTimes(2)

    setVisibility('hidden')
    await vi.advanceTimersByTimeAsync(3 * 60 * 1000)
    expect(usageAPI.getDashboardStats).toHaveBeenCalledTimes(2)

    setVisibility('visible')
    await flushPromises()
    expect(usageAPI.getDashboardStats).toHaveBeenCalledTimes(3)

    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(60 * 1000)
    expect(usageAPI.getDashboardStats).toHaveBeenCalledTimes(3)
  })

  it('取数失败时数值显示破折号，不抛出错误', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.mocked(usageAPI.getDashboardStats).mockRejectedValue(new Error('boom'))
    const wrapper = mount(UserDashboardLiveStats)
    await flushPromises()
    expect(wrapper.get('[data-testid="live-stat-rpm"]').text()).toContain('—')
    wrapper.unmount()
  })
})
