import type { Router, RouteLocationNormalized } from 'vue-router'
import { useNavigationLoadingState } from '@/composables/useNavigationLoading'

// @project-doc docs/architecture/frontend_ui_conventions.md#loading_feedback
/** 将指示器绑定到导航生命周期，取消和异常也走相同的完成路径。 */
export function installNavigationLoading(router: Router): void {
  const loading = useNavigationLoadingState()
  const navigationIds = new WeakMap<RouteLocationNormalized, number>()

  router.beforeEach((to) => {
    navigationIds.set(to, loading.startNavigation())
  })

  const finish = (to: RouteLocationNormalized): void => {
    const id = navigationIds.get(to)
    if (id !== undefined) loading.endNavigation(id)
  }

  router.afterEach(finish)
  router.onError((_error, to) => finish(to))
}
