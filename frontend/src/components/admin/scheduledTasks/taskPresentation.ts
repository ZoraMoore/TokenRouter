import type { ScheduledTaskDraft, ScheduledTaskRunStatus } from '@/api/admin/scheduledTasks'
import { formatDateTime, formatDateTimeToMinute } from '@/utils/format'

export const ACTIVE_RUN_POLL_MS = 2500

export function isActiveRun(status: ScheduledTaskRunStatus | null | undefined): boolean {
  return status === 'queued' || status === 'running'
}

export function runBadge(status: ScheduledTaskRunStatus): string {
  if (status === 'finished') return 'badge-success'
  return isActiveRun(status) ? 'badge-primary' : 'badge-gray'
}

// 更新计划时提交可编辑字段。分组按管理员选中顺序匹配测试模型。
export function taskDraft(form: ScheduledTaskDraft): ScheduledTaskDraft {
  return {
    name: form.name.trim(),
    kind: form.kind,
    scope: form.scope,
    group_ids: form.scope === 'groups' ? [...(form.group_ids ?? [])] : [],
    provider_ids: form.scope === 'providers' ? [...(form.provider_ids ?? [])] : [],
    model: form.model.trim(),
    cron_expression: form.cron_expression.trim().replace(/\s+/g, ' '),
    enabled: form.enabled,
  }
}

// 计划按服务端时区触发，时间列用同一时区显示。旧系统返回 Local 时使用浏览器时间。
export function formatRunTime(value: string, timezone: string): string {
  try {
    return formatDateTime(value, {
      year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
      hour12: false, timeZone: timezone || undefined,
    })
  } catch (error) {
    if (!(error instanceof RangeError)) throw error
    return formatDateTimeToMinute(value)
  }
}
