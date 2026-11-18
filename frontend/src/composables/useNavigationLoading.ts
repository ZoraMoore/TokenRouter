import { computed, readonly, ref } from 'vue'

/** 管理导航指示器，完成动画结束后才移除，快速切页也能得到反馈。 */
export function useNavigationLoading() {
  const phase = ref<'idle' | 'loading' | 'complete'>('idle')
  const navigationId = ref(0)
  let navigationStartTime: number | null = null

  // 每次导航分配独立编号，旧请求和旧动画不能结束后续导航。
  const startNavigation = (): number => {
    navigationId.value += 1
    navigationStartTime = Date.now()
    phase.value = 'loading'
    return navigationId.value
  }

  const endNavigation = (id = navigationId.value): void => {
    if (id !== navigationId.value || phase.value === 'idle') return
    phase.value = 'complete'
    navigationStartTime = null
  }

  const finishNavigation = (id: number): void => {
    if (id !== navigationId.value || phase.value !== 'complete') return
    phase.value = 'idle'
  }

  const resetState = (): void => {
    navigationId.value += 1
    phase.value = 'idle'
    navigationStartTime = null
  }

  const getNavigationDuration = (): number | null => (
    navigationStartTime === null ? null : Date.now() - navigationStartTime
  )

  return {
    isLoading: computed(() => phase.value !== 'idle'),
    isNavigating: computed(() => phase.value === 'loading'),
    navigationId: readonly(navigationId),
    startNavigation,
    endNavigation,
    finishNavigation,
    cancelNavigation: endNavigation,
    resetState,
    getNavigationDuration
  }
}

let navigationLoadingInstance: ReturnType<typeof useNavigationLoading> | null = null

/** 全局导航共用一个指示器实例。 */
export function useNavigationLoadingState() {
  if (!navigationLoadingInstance) {
    navigationLoadingInstance = useNavigationLoading()
  }
  return navigationLoadingInstance
}

/** 清理单例，让各测试独立初始化导航状态。 */
export function _resetNavigationLoadingInstance(): void {
  navigationLoadingInstance?.resetState()
  navigationLoadingInstance = null
}
