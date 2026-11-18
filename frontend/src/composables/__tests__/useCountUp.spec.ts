import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick, ref } from 'vue'
import { mount } from '@vue/test-utils'

import { mockMotionEnvironment } from '@/__tests__/helpers/motion'
import { useCountUp } from '../useCountUp'

// mountCounter 挂载一个只渲染滚动值的组件，返回数值源和显示值。
const mountCounter = (initial: number) => {
  const source = ref(initial)
  let display!: ReturnType<typeof useCountUp>
  const wrapper = mount(defineComponent({
    setup() {
      display = useCountUp(source, 1000)
      return () => h('span', display.value)
    },
  }))
  return { source, display: () => display.value, wrapper }
}

describe('useCountUp', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('初始直接显示当前值，变化后逐帧缓出到新值', async () => {
    mockMotionEnvironment(false)
    let now = 0
    const frames: FrameRequestCallback[] = []
    vi.spyOn(performance, 'now').mockImplementation(() => now)
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      frames.push(callback)
      return frames.length
    })

    const counter = mountCounter(10)
    expect(counter.display()).toBe(10)

    counter.source.value = 110
    await nextTick()
    now = 500
    frames.shift()?.(now)
    // 四次缓出在一半时间时已走过 1 - 0.5^4 = 93.75%
    expect(counter.display()).toBeCloseTo(103.75)

    now = 1000
    frames.shift()?.(now)
    expect(counter.display()).toBe(110)
    expect(frames).toHaveLength(0)
    counter.wrapper.unmount()
  })

  it('系统减少动态效果时直接显示新值', async () => {
    mockMotionEnvironment(true)
    const counter = mountCounter(0)
    counter.source.value = 42
    await nextTick()
    expect(counter.display()).toBe(42)
    counter.wrapper.unmount()
  })

  it('卸载时取消未完成的动画帧', async () => {
    mockMotionEnvironment(false)
    const cancel = vi.spyOn(window, 'cancelAnimationFrame')
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation(() => 7)
    const counter = mountCounter(0)
    counter.source.value = 5
    await nextTick()
    counter.wrapper.unmount()
    expect(cancel).toHaveBeenCalledWith(7)
  })
})
