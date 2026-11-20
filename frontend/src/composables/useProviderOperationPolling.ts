import { ref, watch, type Ref } from 'vue'
import { adminAPI } from '@/api/admin'
import type { ProviderOperation } from '@/api/admin/providers'

const POLL_INTERVAL_MS = 2500
const POLL_CONCURRENCY = 8

// 需要处理的任务由管理员接手，轮询排队中和执行中的任务。
export function isProviderOperationPending(operation: ProviderOperation): boolean {
  return operation.state === 'queued' || operation.state === 'running'
}

// 弹窗可见时刷新任务，收起后取消请求和计时器。
export function useProviderOperationPolling(operations: Ref<ProviderOperation[]>, enabled: () => boolean) {
  const refreshFailed = ref(false)

  watch(() => enabled() && operations.value.some(isProviderOperationPending), (active, _previous, onCleanup) => {
    refreshFailed.value = false
    if (!active) return

    const controller = new AbortController()
    let timer: ReturnType<typeof setTimeout> | undefined
    onCleanup(() => {
      controller.abort()
      clearTimeout(timer)
    })

    const poll = async () => {
      const pending = operations.value
        .map((operation, index) => ({ operation, index }))
        .filter(({ operation }) => isProviderOperationPending(operation))
      let failed = false
      // 跨页重登可以选中大量账号，分批读取任务以限制并发请求数。
      for (let offset = 0; offset < pending.length && !controller.signal.aborted; offset += POLL_CONCURRENCY) {
        const batch = pending.slice(offset, offset + POLL_CONCURRENCY)
        const results = await Promise.allSettled(batch.map(({ operation }) => adminAPI.providers.getOperation(operation.id, controller.signal)))
        if (controller.signal.aborted) return
        results.forEach((result, position) => {
          const { operation, index } = batch[position]
          if (result.status === 'rejected') {
            failed = true
          } else if (operations.value[index]?.id === operation.id) {
            operations.value[index] = result.value
          }
        })
      }
      refreshFailed.value = failed
      if (operations.value.some(isProviderOperationPending)) timer = setTimeout(poll, POLL_INTERVAL_MS)
    }

    void poll()
  }, { immediate: true })

  return refreshFailed
}
