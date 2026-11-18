import { ref, shallowRef } from 'vue'
import { getPelicanVisibility, type PelicanVisibility } from '@/api/pelicanShowcase'

export const PELICAN_VISIBILITY_INTERVAL = 30_000
export const pelicanViewerId = ref<number | null>(null)
export const pelicanVisibility = shallowRef<PelicanVisibility | null>(null)
let checkedAt = 0
let pending: { controller: AbortController; promise: Promise<PelicanVisibility | null> } | null = null

function cancelRead() {
  pending?.controller.abort()
  pending = null
}

export function clearPelicanVisibility() {
  cancelRead()
  pelicanVisibility.value = null
  pelicanViewerId.value = null
  checkedAt = 0
}

// PUT 成功后，侧栏和 API 说明立即使用服务端确认的值。
export function setPelicanVisibility(settings: PelicanVisibility) {
  cancelRead()
  pelicanVisibility.value = { enabled: settings.enabled === true, api_enabled: settings.enabled === true && settings.api_enabled === true }
  checkedAt = Date.now()
}

// 路由守卫和侧栏共享同一次读取。账号变更时撤销旧请求，旧响应无法写入新会话。
export function readPelicanVisibility(userId: number, force = false): Promise<PelicanVisibility | null> {
  if (pelicanViewerId.value !== userId) {
    clearPelicanVisibility()
    pelicanViewerId.value = userId
  }
  if (pending) return pending.promise
  if (!force && pelicanVisibility.value && Date.now() - checkedAt < PELICAN_VISIBILITY_INTERVAL) return Promise.resolve(pelicanVisibility.value)
  const controller = new AbortController()
  const promise = getPelicanVisibility(controller.signal).then(settings => {
    if (!controller.signal.aborted) {
      pelicanVisibility.value = { enabled: settings.enabled === true, api_enabled: settings.enabled === true && settings.api_enabled === true }
      checkedAt = Date.now()
    }
    return pelicanViewerId.value === userId ? pelicanVisibility.value : null
  }).catch(() => controller.signal.aborted && pelicanViewerId.value === userId ? pelicanVisibility.value : null).finally(() => {
    if (pending?.controller === controller) pending = null
  })
  pending = { controller, promise }
  return promise
}
