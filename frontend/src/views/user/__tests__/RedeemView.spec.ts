import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { createI18n, type MessageContext } from 'vue-i18n'
import RedeemView from '../RedeemView.vue'
import zh from '@/i18n/locales/zh/dashboard'
import en from '@/i18n/locales/en/dashboard'
import { mockMotionEnvironment } from '@/__tests__/helpers/motion'

const mocks = vi.hoisted(() => ({
  redeem: vi.fn(),
  getHistory: vi.fn(),
  refreshUser: vi.fn(),
  fetchActiveSubscriptions: vi.fn(),
  app: {
    showError: vi.fn(),
    showWarning: vi.fn(),
    showSuccess: vi.fn(),
    cachedPublicSettings: { balance_unit_symbol: '积分 ' }
  }
}))

vi.mock('@/api', () => ({ redeemAPI: mocks }))
vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({ user: { balance: 10, concurrency: 2 }, refreshUser: mocks.refreshUser })
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => mocks.app }))
vi.mock('@/stores', () => ({ useAppStore: () => mocks.app }))
vi.mock('@/stores/subscriptions', () => ({
  useSubscriptionStore: () => ({ activeSubscriptions: [], fetchActiveSubscriptions: mocks.fetchActiveSubscriptions })
}))
vi.mock('@/i18n', () => ({ getLocale: () => 'zh-CN' }))
vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<main><slot /></main>' }
}))

let wrapper: VueWrapper | undefined

// 项目使用不含编译器的 i18n 运行时，测试以消息函数读取真实词条和插值。
function testMessages(messages: { redeem: Record<string, string> }) {
  return {
    redeem: Object.fromEntries(Object.entries(messages.redeem).map(([key, value]) => [
      key,
      (context: MessageContext) => value.replace(/\{(\w+)\}/g, (_match, name) => String(context.named(name)))
    ])),
    subscriptionProgress: {
      title: () => '我的订阅',
      noSubscriptions: () => '暂无订阅'
    }
  }
}

async function mountView(locale = 'zh') {
  wrapper = mount(RedeemView, {
    attachTo: document.body,
    global: {
      plugins: [createPinia(), createI18n({ legacy: false, locale, messages: { zh: testMessages(zh), en: testMessages(en) } })],
      stubs: {
        transition: true,
        AppLayout: { template: '<main><slot /></main>' },
        Icon: true,
        BalanceIcon: true,
        Pagination: true,
        SubscriptionUsageList: true,
        RouterLink: true
      }
    }
  })
  await flushPromises()
  return wrapper
}

async function submit(code = 'GIFT-CODE') {
  await wrapper!.get('input').setValue(code)
  await wrapper!.get('form').trigger('submit')
  await flushPromises()
}

beforeEach(() => {
  vi.clearAllMocks()
  mockMotionEnvironment()
  mocks.redeem.mockReset().mockResolvedValue({ type: 'balance', value: 12.5 })
  mocks.getHistory.mockReset().mockResolvedValue({ items: [], total: 0 })
  mocks.refreshUser.mockReset().mockResolvedValue({ balance: 22.5, concurrency: 2 })
  mocks.fetchActiveSubscriptions.mockReset().mockResolvedValue([])
})

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('兑换成功反馈', () => {
  it.each([
    ['balance', 12.5, '余额已到账 +积分12.50'],
    ['concurrency', 3, '并发数 +3'],
    ['subscription', 0, '订阅已领取']
  ])('展示 %s 权益，清空输入并刷新数据，不重复弹成功 Toast', async (type, value, detail) => {
    mocks.redeem.mockResolvedValue({ type, value })
    await mountView()
    expect(wrapper!.find('[data-testid="redeem-celebration"]').exists()).toBe(false)
    await submit('  GIFT-CODE  ')

    expect(mocks.redeem).toHaveBeenCalledOnce()
    expect(mocks.redeem).toHaveBeenCalledWith('GIFT-CODE')
    expect(wrapper!.get('[data-testid="redeem-celebration"]').text()).toContain(detail)
    expect(wrapper!.get('[role="status"]').text()).toContain(detail)
    expect((wrapper!.get('input').element as HTMLInputElement).value).toBe('')
    expect((wrapper!.get('input').element as HTMLInputElement).disabled).toBe(false)
    expect(mocks.refreshUser).toHaveBeenCalledOnce()
    expect(mocks.getHistory).toHaveBeenLastCalledWith(1, 10)
    expect(mocks.fetchActiveSubscriptions.mock.calls).toEqual(type === 'subscription' ? [[], [true]] : [[]])
    expect(mocks.app.showSuccess).not.toHaveBeenCalled()
    expect(mocks.app.showError).not.toHaveBeenCalled()
    expect(mocks.app.showWarning).not.toHaveBeenCalled()
  })

  it('英文成功提示也包含实际到账金额和自定义单位', async () => {
    await mountView('en')
    await submit()
    expect(wrapper!.get('[role="status"]').text()).toContain('Balance credited +积分12.50')
  })

  it('接口确认后立即庆祝，不等待用户资料刷新', async () => {
    let finishRefresh!: () => void
    mocks.refreshUser.mockImplementation(() => new Promise<void>((resolve) => { finishRefresh = resolve }))
    await mountView()
    await submit()
    expect(wrapper!.find('[data-testid="redeem-celebration"]').exists()).toBe(true)
    expect(mocks.getHistory).toHaveBeenCalledTimes(2)
    finishRefresh()
    await flushPromises()
    expect((wrapper!.get('input').element as HTMLInputElement).disabled).toBe(false)
  })

  it.each(['user', 'history', 'subscription'])('%s 刷新失败仍保留兑换成功，只提示刷新异常', async (target) => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    await mountView()
    mocks.redeem.mockResolvedValue({ type: 'subscription', value: 0 })
    const action = target === 'user' ? mocks.refreshUser : target === 'history' ? mocks.getHistory : mocks.fetchActiveSubscriptions
    action.mockRejectedValueOnce(new Error('offline'))
    await submit()
    expect(wrapper!.get('[role="status"]').text()).toContain('订阅已领取')
    expect(mocks.app.showWarning).toHaveBeenCalledOnce()
    expect(mocks.app.showWarning).toHaveBeenCalledWith(zh.redeem.dataRefreshFailed)
    expect(mocks.app.showError).not.toHaveBeenCalled()
    expect(mocks.refreshUser).toHaveBeenCalledOnce()
    expect(mocks.getHistory).toHaveBeenCalledTimes(2)
    expect(mocks.fetchActiveSubscriptions).toHaveBeenLastCalledWith(true)
  })

  it('兑换失败保留输入并提示错误，不庆祝或刷新数据', async () => {
    await mountView()
    mocks.redeem.mockRejectedValue(new Error('offline'))
    await submit()
    expect(wrapper!.find('[data-testid="redeem-celebration"]').exists()).toBe(false)
    expect((wrapper!.get('input').element as HTMLInputElement).value).toBe('GIFT-CODE')
    expect(mocks.app.showError).toHaveBeenCalledOnce()
    expect(mocks.refreshUser).not.toHaveBeenCalled()
    expect(mocks.getHistory).toHaveBeenCalledOnce()
  })

  it('请求进行中重复提交只发起一次兑换', async () => {
    let finishRedeem!: (value: unknown) => void
    mocks.redeem.mockImplementation(() => new Promise((resolve) => { finishRedeem = resolve }))
    await mountView()
    await submit()
    await wrapper!.get('form').trigger('submit')
    expect(mocks.redeem).toHaveBeenCalledOnce()
    finishRedeem({ type: 'balance', value: 1 })
    await flushPromises()
  })

  it('下一次提交清除旧反馈，失败时不会保留上一次庆祝', async () => {
    await mountView()
    await submit()
    mocks.redeem.mockRejectedValueOnce(new Error('offline'))
    await submit('NEXT-CODE')
    expect(wrapper!.find('[data-testid="redeem-celebration"]').exists()).toBe(false)
    expect(wrapper!.get('[role="status"]').text()).toBe('')
  })

  it('连续成功兑换不会被上一次提示的计时器提前关闭', async () => {
    await mountView()
    vi.useFakeTimers()
    await submit()
    await vi.advanceTimersByTimeAsync(2000)
    mocks.redeem.mockResolvedValueOnce({ type: 'concurrency', value: 4 })
    await submit('NEXT-CODE')
    await vi.advanceTimersByTimeAsync(1100)
    expect(wrapper!.get('[data-testid="redeem-celebration"]').text()).toContain('并发数 +4')
    await vi.advanceTimersByTimeAsync(1900)
    expect(wrapper!.find('[data-testid="redeem-celebration"]').exists()).toBe(false)
  })

  it('离开页面后到达的兑换响应不会触发反馈或额外刷新', async () => {
    let finishRedeem!: (value: unknown) => void
    mocks.redeem.mockImplementation(() => new Promise((resolve) => { finishRedeem = resolve }))
    await mountView()
    await submit()
    wrapper!.unmount()
    wrapper = undefined
    finishRedeem({ type: 'balance', value: 1 })
    await flushPromises()
    expect(mocks.refreshUser).not.toHaveBeenCalled()
    expect(mocks.app.showError).not.toHaveBeenCalled()
    expect(mocks.app.showWarning).not.toHaveBeenCalled()
  })

  it.each([false, true])('提交后恢复输入焦点，但不抢走用户移开的焦点（移开：%s）', async (moveFocus) => {
    let finishRefresh!: () => void
    mocks.refreshUser.mockImplementation(() => new Promise<void>((resolve) => { finishRefresh = resolve }))
    await mountView()
    const input = wrapper!.get('input').element as HTMLInputElement
    const otherControl = document.createElement('button')
    document.body.append(otherControl)
    try {
      input.focus()
      await submit()
      // jsdom 不会因 disabled 自动失焦，显式模拟浏览器的行为。
      input.blur()
      if (moveFocus) otherControl.focus()
      finishRefresh()
      await flushPromises()
      expect(document.activeElement).toBe(moveFocus ? otherControl : input)
    } finally {
      otherControl.remove()
    }
  })
})

// 付款资格错误保留兑换码，付款后可以再次提交。
describe('付款领取条件', () => {
  it.each(['zh', 'en'])('按 %s 展示未付款提示', async (locale) => {
    mocks.redeem.mockRejectedValue({ status: 403, reason: 'REDEEM_PAYMENT_REQUIRED' })
    await mountView(locale)
    await submit()
    expect(mocks.app.showError).toHaveBeenCalledWith(
      locale === 'zh' ? zh.redeem.paymentRequired : en.redeem.paymentRequired
    )
    expect((wrapper!.get('input').element as HTMLInputElement).value).toBe('GIFT-CODE')
    expect(wrapper!.find('[data-testid="redeem-celebration"]').exists()).toBe(false)
    expect(mocks.refreshUser).not.toHaveBeenCalled()
  })
})
