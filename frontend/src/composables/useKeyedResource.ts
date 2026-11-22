import { ref, shallowRef, watch } from 'vue'

// 切换条目时撤销上次读取。同一条目刷新期间继续显示已加载的数据。
export function useKeyedResource<K, T>(
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
        // 请求结束后才开始计时。保留同一条目现有数据的同时放慢失败后的轮询。
        const delay = data.value === null ? undefined : pollDelay?.(data.value)
        if (delay !== undefined) {
          timer = window.setTimeout(refresh, failed.value ? Math.max(delay, 10_000) : delay)
        }
      }
    }
  }, { immediate: true })

  function refresh() {
    revision.value++
  }

  return { data, loading, failed, refresh }
}
