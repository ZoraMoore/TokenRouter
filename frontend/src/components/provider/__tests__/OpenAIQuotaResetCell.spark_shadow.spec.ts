import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import OpenAIQuotaResetCell from '../OpenAIQuotaResetCell.vue'
import type { Provider } from '@/types'
import { refreshOpenAIQuota, resetOpenAIQuota } from '@/api/admin/providers'

vi.mock('@/api/admin/providers', () => ({
  refreshOpenAIQuota: vi.fn(),
  resetOpenAIQuota: vi.fn(),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string) => key }),
  }
})

function makeProvider(overrides: Partial<Provider>): Provider {
  return {
    id: 1,
    name: 'acc',
    platform: 'openai',
    type: 'oauth',
    proxy_id: null,
    concurrency: 3,
    priority: 50,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides,
  }
}

beforeEach(() => {
  vi.mocked(refreshOpenAIQuota).mockReset()
})

describe('OpenAIQuotaResetCell', () => {
  it('影子提供商(parent_provider_id 非空)只展示查询按钮，不展示重置入口', () => {
    const provider = makeProvider({ parent_provider_id: 100 })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { provider } })

    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(1)
    expect(buttons[0].attributes('title')).toBe('admin.providers.openaiQuotaReset.countTooltipLoad')
    wrapper.unmount()
  })

  it('母提供商可以打开邀请弹窗，查到次数前禁用重置', async () => {
    const provider = makeProvider({ parent_provider_id: null })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { provider } })

    const buttons = wrapper.findAll('button')
    expect(buttons).toHaveLength(3)
    expect(buttons[0].attributes('title')).toBe('admin.providers.openaiQuotaReset.countTooltipLoad')
    expect(wrapper.get('[data-testid="quota-reset"]').attributes('disabled')).toBeDefined()
    await wrapper.get('[data-testid="quota-invite"]').trigger('click')
    expect(wrapper.emitted('invite-reset')).toEqual([[provider]])
    wrapper.unmount()
  })

  it('从提供商 extra 恢复未过期的次数和到期明细', () => {
    const provider = makeProvider({
      extra: {
        codex_reset_credit_snapshot: {
          available_count: 2,
          credits: [
            { expires_at: '2099-07-05T04:05:06Z' },
            { expires_at: '2099-07-03T04:05:06Z' },
          ],
        },
      },
    })

    const wrapper = mount(OpenAIQuotaResetCell, { props: { provider } })

    expect(refreshOpenAIQuota).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('admin.providers.openaiQuotaReset.count2')
    expect(wrapper.text()).toContain('admin.providers.openaiQuotaReset.expiresAt')
    expect(wrapper.text()).toContain('+1')
    // 明细按钮用于展开到期信息。
    expect(wrapper.findAll('button')).toHaveLength(4)
    wrapper.unmount()
  })

  it('缓存次数全部过期时视为未知', () => {
    const provider = makeProvider({
      extra: {
        codex_reset_credit_snapshot: {
          available_count: 1,
          credits: [{ expires_at: '2020-07-03T04:05:06Z' }],
        },
      },
    })

    const wrapper = mount(OpenAIQuotaResetCell, { props: { provider } })

    expect(wrapper.text()).not.toContain('admin.providers.openaiQuotaReset.count1')
    expect(wrapper.text()).not.toContain('admin.providers.openaiQuotaReset.expiresAt')
    expect(wrapper.get('[data-testid="quota-reset"]').attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })

  it('快照持久化失败时仍显示实时次数并告警', async () => {
    vi.mocked(refreshOpenAIQuota).mockResolvedValue({
      rate_limit_reset_credits: { available_count: 2 },
      fetched_at: 1770000000,
      cache_persisted: false,
    })
    const wrapper = mount(OpenAIQuotaResetCell, {
      props: { provider: makeProvider({ parent_provider_id: null }) },
    })

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(refreshOpenAIQuota).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('admin.providers.openaiQuotaReset.count2')
    expect(wrapper.text()).toContain('admin.providers.openaiQuotaReset.refreshCachePersistFailed')
    expect(wrapper.get('[data-testid="quota-reset"]').attributes('disabled')).toBeUndefined()
    wrapper.unmount()
  })

  it('确认一次只消费一次，使用响应里的最新窗口和次数', async () => {
    const quota = { fetched_at: 4, rate_limit_reset_credits: { available_count: 0 } }
    vi.mocked(resetOpenAIQuota).mockResolvedValue({ code: 'reset', windows_reset: 2, quota, cache_refreshed: true, provider_state_recovered: true })
    const provider = makeProvider({ extra: { codex_reset_credit_snapshot: { available_count: 1, credits: [{ expires_at: '2099-07-03T04:05:06Z' }] } } })
    const wrapper = mount(OpenAIQuotaResetCell, { props: { provider }, global: { stubs: { ConfirmDialog: { name: 'ConfirmDialog', props: ['show'], emits: ['confirm'], template: '<span />' } } } })
    await wrapper.get('[data-testid="quota-reset"]').trigger('click')
    const dialog = wrapper.getComponent({ name: 'ConfirmDialog' })
    expect(dialog.props('show')).toBe(true)
    dialog.vm.$emit('confirm')
    dialog.vm.$emit('confirm')
    await flushPromises()
    expect(resetOpenAIQuota).toHaveBeenCalledTimes(1)
    expect(resetOpenAIQuota).toHaveBeenCalledWith(provider.id)
    expect(wrapper.emitted('quota-updated')).toEqual([[quota]])
    expect(wrapper.get('[data-testid="quota-reset"]').attributes('disabled')).toBeDefined()
  })
})
