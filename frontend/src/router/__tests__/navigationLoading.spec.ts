import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createMemoryHistory, createRouter } from 'vue-router'
import { installNavigationLoading } from '../navigationLoading'
import { _resetNavigationLoadingInstance, useNavigationLoadingState } from '@/composables/useNavigationLoading'

beforeEach(_resetNavigationLoadingInstance)
afterEach(_resetNavigationLoadingInstance)

function setupRouter() {
  const view = { render: () => null }
  const router = createRouter({
    history: createMemoryHistory(),
    routes: ['/', '/slow', '/newer', '/error', '/blocked', '/redirect'].map(path => ({ path, component: view }))
  })
  installNavigationLoading(router)
  return router
}

function deferred() {
  let resolve!: () => void
  const promise = new Promise<void>(done => { resolve = done })
  return { promise, resolve }
}

describe('路由进度生命周期', () => {
  it('导航异常和守卫中止都会结束加载', async () => {
    const router = setupRouter()
    const state = useNavigationLoadingState()
    router.beforeEach(to => {
      if (to.path === '/error') throw new Error('导航失败')
      if (to.path === '/blocked') return false
    })
    await expect(router.push('/error')).rejects.toThrow('导航失败')
    expect(state.isNavigating.value).toBe(false)
    state.finishNavigation(state.navigationId.value)
    expect(state.isLoading.value).toBe(false)
    await router.push('/blocked')
    expect(state.isNavigating.value).toBe(false)
  })

  it('取消慢导航后，其完成回调不会关闭新导航', async () => {
    const router = setupRouter()
    const state = useNavigationLoadingState()
    const slow = deferred()
    const newer = deferred()
    router.beforeEach(to => {
      if (to.path === '/slow') return slow.promise
      if (to.path === '/newer') return newer.promise
    })
    const first = router.push('/slow')
    await vi.waitFor(() => expect(state.isNavigating.value).toBe(true))
    const firstId = state.navigationId.value
    const second = router.push('/newer')
    await vi.waitFor(() => expect(state.navigationId.value).toBeGreaterThan(firstId))
    slow.resolve()
    await first
    expect(state.isNavigating.value).toBe(true)
    newer.resolve()
    await second
    expect(state.isNavigating.value).toBe(false)
  })

  it('重定向完成后进入收尾状态', async () => {
    const router = setupRouter()
    const state = useNavigationLoadingState()
    router.beforeEach(to => to.path === '/redirect' ? '/' : undefined)
    await router.push('/redirect')
    expect(router.currentRoute.value.path).toBe('/')
    expect(state.isNavigating.value).toBe(false)
    expect(state.isLoading.value).toBe(true)
  })
})
