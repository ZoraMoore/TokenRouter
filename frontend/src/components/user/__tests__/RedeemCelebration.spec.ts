import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { mount, type VueWrapper } from '@vue/test-utils'
import RedeemCelebration from '../RedeemCelebration.vue'
import { finishMotion, mockMotionEnvironment } from '@/__tests__/helpers/motion'

let wrapper: VueWrapper | undefined

async function mountCelebration(reduced = false, realTransition = false) {
  const environment = mockMotionEnvironment(reduced)
  wrapper = mount(RedeemCelebration, {
    props: { sequence: 0, title: '兑换成功！', detail: '余额已到账 +$12.50' },
    slots: { default: '<dl><dt>当前余额</dt><dd>$22.50</dd></dl>' },
    global: { stubs: { Icon: true, transition: !realTransition } }
  })
  await wrapper.setProps({ sequence: 1 })
  return environment
}

afterEach(() => {
  wrapper?.unmount()
  wrapper = undefined
  vi.restoreAllMocks()
  vi.useRealTimers()
})

describe('RedeemCelebration', () => {
  it('保留统计占位，退出动画完成后才恢复显示', async () => {
    await mountCelebration(false, true)
    const statistics = wrapper!.get('dl').element.parentElement!
    expect(statistics.classList.contains('invisible')).toBe(true)
    expect(statistics.getAttribute('aria-hidden')).toBe('true')
    const feedback = wrapper!.get('[data-testid="redeem-celebration"]').element
    await wrapper!.setProps({ sequence: 0 })
    expect(statistics.classList.contains('invisible')).toBe(true)
    await finishMotion(feedback)
    expect(wrapper!.find('[data-testid="redeem-celebration"]').exists()).toBe(false)
    expect(statistics.classList.contains('invisible')).toBe(false)
    expect(statistics.hasAttribute('aria-hidden')).toBe(false)
  })

  it('粒子动画完成时移除装饰，保留成功文案且不因文案变化重播', async () => {
    await mountCelebration()
    expect(wrapper!.findAll('.redeem-confetti-piece')).toHaveLength(16)
    await wrapper!.get('.redeem-confetti-piece').trigger('animationend')
    expect(wrapper!.find('.redeem-confetti').exists()).toBe(true)
    await wrapper!.get('.redeem-confetti').trigger('animationend')
    expect(wrapper!.find('.redeem-confetti').exists()).toBe(false)
    expect(wrapper!.get('[role="status"]').text()).toContain('余额已到账 +$12.50')
    await wrapper!.setProps({ detail: '其他内容' })
    expect(wrapper!.find('.redeem-confetti').exists()).toBe(false)
    expect(wrapper!.get('[role="status"]').text()).toContain('余额已到账 +$12.50')
  })

  it('旧反馈的退出完成事件不能清除新一次成功', async () => {
    await mountCelebration(false, true)
    const previous = wrapper!.get('[data-testid="redeem-celebration"]').element
    await wrapper!.setProps({ sequence: 0 })
    await wrapper!.setProps({ sequence: 2, detail: '并发数 +3' })
    await finishMotion(previous)
    expect(wrapper!.get('[data-testid="redeem-celebration"]').text()).toContain('并发数 +3')
    expect(wrapper!.get('dl').element.parentElement!.classList.contains('invisible')).toBe(true)
  })

  it('减少动态效果时展示静态信息，保持相同阅读时间', async () => {
    vi.useFakeTimers()
    await mountCelebration(true)
    expect(wrapper!.find('.redeem-confetti').exists()).toBe(false)
    expect(wrapper!.find('.redeem-success-pop').exists()).toBe(false)
    await vi.advanceTimersByTimeAsync(2999)
    expect(wrapper!.find('[data-testid="redeem-celebration"]').exists()).toBe(true)
    await vi.advanceTimersByTimeAsync(1)
    expect(wrapper!.find('[data-testid="redeem-celebration"]').exists()).toBe(false)
  })

  it('播放期间开启减少动态效果立即停止，关闭该偏好也不重播', async () => {
    const environment = await mountCelebration()
    environment.reduce(true)
    await nextTick()
    expect(wrapper!.find('.redeem-confetti').exists()).toBe(false)
    expect(wrapper!.find('.redeem-success-pop').exists()).toBe(false)
    expect(wrapper!.get('[role="status"]').text()).toContain('兑换成功！')
    environment.reduce(false)
    await nextTick()
    expect(wrapper!.find('.redeem-confetti').exists()).toBe(false)
  })

  it('卸载时清理反馈定时器和媒体查询监听', async () => {
    vi.useFakeTimers()
    const environment = await mountCelebration()
    expect(vi.getTimerCount()).toBe(1)
    wrapper!.unmount()
    wrapper = undefined
    expect(vi.getTimerCount()).toBe(0)
    expect(environment.listeners.size).toBe(0)
  })
})
