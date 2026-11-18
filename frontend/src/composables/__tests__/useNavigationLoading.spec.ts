import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useNavigationLoading } from '../useNavigationLoading'

describe('导航加载反馈', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => vi.useRealTimers())

  it('导航开始立即可见，快速完成后等动画结束再隐藏', () => {
    const state = useNavigationLoading()
    const id = state.startNavigation()
    expect(state.isLoading.value).toBe(true)
    expect(state.isNavigating.value).toBe(true)

    state.endNavigation(id)
    expect(state.isLoading.value).toBe(true)
    expect(state.isNavigating.value).toBe(false)
    state.finishNavigation(id)
    expect(state.isLoading.value).toBe(false)
  })

  it('旧导航的完成回调不能结束新的导航', () => {
    const state = useNavigationLoading()
    const first = state.startNavigation()
    const second = state.startNavigation()
    state.endNavigation(first)
    expect(state.isNavigating.value).toBe(true)
    state.endNavigation(second)
    expect(state.isNavigating.value).toBe(false)
  })

  it('旧完成动画不能隐藏下一次导航', () => {
    const state = useNavigationLoading()
    const first = state.startNavigation()
    state.endNavigation(first)
    const second = state.startNavigation()
    state.finishNavigation(first)
    expect(state.isLoading.value).toBe(true)
    state.endNavigation(second)
    state.finishNavigation(first)
    expect(state.isLoading.value).toBe(true)
    state.finishNavigation(second)
    expect(state.isLoading.value).toBe(false)
  })

  it('取消导航也能正常收尾', () => {
    const state = useNavigationLoading()
    const id = state.startNavigation()
    state.cancelNavigation(id)
    expect(state.isNavigating.value).toBe(false)
    state.finishNavigation(id)
    expect(state.isLoading.value).toBe(false)
  })

  it('只记录正在进行的导航时长，重置使旧回调失效', () => {
    const state = useNavigationLoading()
    expect(state.getNavigationDuration()).toBeNull()
    const id = state.startNavigation()
    vi.advanceTimersByTime(500)
    expect(state.getNavigationDuration()).toBe(500)
    state.endNavigation(id)
    expect(state.getNavigationDuration()).toBeNull()
    state.resetState()
    state.endNavigation(id)
    expect(state.isLoading.value).toBe(false)
  })
})
