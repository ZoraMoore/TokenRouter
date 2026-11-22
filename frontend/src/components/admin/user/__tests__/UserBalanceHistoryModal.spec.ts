import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { AdminUser } from '@/types'
import UserBalanceHistoryModal from '../UserBalanceHistoryModal.vue'

const { getUserBalanceHistory } = vi.hoisted(() => ({
  getUserBalanceHistory: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: { users: { getUserBalanceHistory } }
}))

vi.mock('@/composables/useBalanceDisplay', () => ({
  useBalanceDisplay: () => ({ formatBalanceAmount: (value: number) => String(value) })
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...await importOriginal<typeof import('vue-i18n')>(),
  useI18n: () => ({
    t: (key: string, values?: { minimum: string; discount: string; currency: string }) =>
      values ? `${key} ${values.minimum} ${values.discount} ${values.currency}` : key
  })
}))

const user = {
  id: 99,
  email: 'user@example.com',
  balance: 20,
  created_at: '2026-11-19T00:00:00Z'
} as AdminUser

// 优惠券通过 coupon_terms 返回满减金额，value 为零。
const couponRecord = {
  id: 1,
  code: 'RECHARGE-COUPON',
  type: 'coupon',
  value: 0,
  created_at: user.created_at,
  coupon_terms: { currency: 'CNY', minimum_minor: 3000, discount_minor: 500 }
}

// 打开弹窗后由组件加载记录。
async function openHistory() {
  const wrapper = mount(UserBalanceHistoryModal, {
    props: { show: false, user, hideActions: true },
    global: {
      stubs: {
        BaseDialog: { template: '<div><slot /></div>' },
        UserAvatar: true,
        BalanceIcon: true,
        Teleport: true
      }
    }
  })

  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

describe('UserBalanceHistoryModal', () => {
  beforeEach(() => {
    getUserBalanceHistory.mockReset()
    getUserBalanceHistory.mockResolvedValue({ items: [couponRecord], total: 1, total_recharged: 20 })
  })

  it('显示已领取的优惠券和满减金额', async () => {
    const wrapper = await openHistory()

    expect(wrapper.text()).toContain('redeem.couponReceived')
    expect(wrapper.text()).toMatch(/payment\.coupons\.terms \D*30\.00 \D*5\.00 CNY/)
    expect(wrapper.text()).not.toContain('common.unknown')
    expect(wrapper.text()).not.toContain('+0')
    wrapper.unmount()
  })

  it('选择优惠券类型后按 coupon 请求第一页', async () => {
    const wrapper = await openHistory()
    await wrapper.get('button[aria-haspopup="true"]').trigger('click')
    const option = wrapper.findAll('[role="option"]').find((item) => item.text() === 'admin.users.typeCoupon')

    expect(option).toBeDefined()
    await option!.trigger('click')
    await flushPromises()
    expect(getUserBalanceHistory).toHaveBeenLastCalledWith(user.id, 1, 15, 'coupon')
    wrapper.unmount()
  })

  it('缺少券面数据时仍识别优惠券类型', async () => {
    getUserBalanceHistory.mockResolvedValue({
      items: [{ ...couponRecord, coupon_terms: null }],
      total: 1,
      total_recharged: 20
    })
    const wrapper = await openHistory()

    expect(wrapper.text()).toContain('redeem.couponReceived')
    expect(wrapper.text()).not.toContain('+0')
    wrapper.unmount()
  })
})
