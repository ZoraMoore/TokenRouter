import { ref, shallowRef, watch } from 'vue'
import type { PelicanModel } from '@/api/pelicanTests'

export const PELICAN_SLOW_POLL_MS = 30_000
type PollablePlan = Pick<PelicanModel, 'run_state' | 'enabled'> & Partial<Pick<PelicanModel, 'latest' | 'artworks'>>

// 等待定时采样或人工复核时每 30 秒刷新，正在排队或作答时缩短间隔。
export function pelicanPollDelay(plans: readonly PollablePlan[]): number | undefined {
  if (plans.some(plan => plan.run_state === 'queued' || plan.run_state === 'running')) return 2500
  if (plans.some(plan => plan.enabled || plan.latest?.status === 'pending_review' ||
    plan.artworks?.some(art => art.status === 'pending_review'))) return PELICAN_SLOW_POLL_MS
}

// 切换分组、计划或样本时撤销上次读取。同一项刷新期间继续显示已加载的数据。
export function usePelicanResource<K, T>(
  keyFor: () => K | null,
  read: (key: K, signal: AbortSignal) => Promise<T>,
  pollDelay?: (data: T) => number | undefined,
) {
  const data = shallowRef<T | null>(null)
  const loading = ref(false)
  const failed = ref(false)
  const revision = ref(0)

  watch([keyFor, revision], async ([key], previous, onCleanup) => {
    if (key !== previous?.[0]) data.value = null
    failed.value = false
    loading.value = key !== null
    if (key === null) return

    const controller = new AbortController()
    let timer: number | undefined
    onCleanup(() => {
      controller.abort()
      window.clearTimeout(timer)
    })
    try {
      const result = await read(key, controller.signal)
      if (!controller.signal.aborted) data.value = result
    } catch {
      if (!controller.signal.aborted) failed.value = true
    } finally {
      if (!controller.signal.aborted) {
        loading.value = false
        // 每轮请求结束后才开始计时，读取失败时放慢重试。
        const delay = data.value === null ? undefined : pollDelay?.(data.value)
        if (delay !== undefined) {
          timer = window.setTimeout(refresh, failed.value ? Math.max(delay, 10000) : delay)
        }
      }
    }
  }, { immediate: true })

  function refresh() {
    revision.value++
  }

  return { data, loading, failed, refresh }
}
