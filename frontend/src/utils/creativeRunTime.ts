// 创作任务的时间换算：后端时间戳可能是秒或毫秒，历史面板和输入框状态行共用这里的耗时格式。
import { CREATIVE_RUN_TERMINAL_STATUSES, type CreativeRun } from '@/api/creative'

// 小于 1e12 的时间戳按秒处理，换算成毫秒；空值和非有限数返回 null。
export function creativeTimestampToMs(timestamp: number | null | undefined): number | null {
  if (timestamp == null || !Number.isFinite(timestamp)) return null
  return timestamp < 1e12 ? timestamp * 1000 : timestamp
}

// 返回任务耗时 `mm:ss`，超过一小时为 `hh:mm:ss`。
// 进行中的任务算到 now；终态任务算到完成或取消时间，缺少结束时间时返回空字符串。
export function formatCreativeRunElapsed(run: CreativeRun, now: number): string {
  const startedAt = creativeTimestampToMs(run.started_at ?? run.created_at)
  if (startedAt == null) return ''
  const endedAt = CREATIVE_RUN_TERMINAL_STATUSES.includes(run.status)
    ? creativeTimestampToMs(run.completed_at ?? run.cancelled_at)
    : now
  if (endedAt == null) return ''
  const totalSeconds = Math.max(0, Math.floor((endedAt - startedAt) / 1000))
  const seconds = totalSeconds % 60
  const minutes = Math.floor(totalSeconds / 60) % 60
  const hours = Math.floor(totalSeconds / 3600)
  const pad = (value: number): string => String(value).padStart(2, '0')
  return hours > 0 ? `${pad(hours)}:${pad(minutes)}:${pad(seconds)}` : `${pad(minutes)}:${pad(seconds)}`
}
