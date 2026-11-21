import { apiClient } from '../client'

export type ScheduledTaskKind = 'relogin' | 'iq'
export type ScheduledTaskScope = 'all' | 'groups' | 'providers'
export type ScheduledTaskRunStatus = 'queued' | 'running' | 'finished' | 'cancelled'

// 计划的范围由 scope 决定，另外两种范围的数据不会随表单提交。
export interface ScheduledTaskDraft {
  name: string
  kind: ScheduledTaskKind
  scope: ScheduledTaskScope
  group_ids: number[]
  provider_ids: number[]
  model: string
  cron_expression: string
  enabled: boolean
}

export interface ScheduledTaskPlan extends ScheduledTaskDraft {
  id: number
  next_run_at?: string | null
  last_run_id?: number | null
  last_run_at?: string | null
  last_run_status?: ScheduledTaskRunStatus | null
}

export interface ScheduledTaskTarget {
  provider_id: number
  provider_name: string
  group_id?: number
  model?: string
  actual_model?: string
  reason?: string
}

export interface ScheduledTaskPreview {
  items: ScheduledTaskTarget[]
  total: number
  eligible: number
}

export interface ScheduledTaskRun {
  id: number
  plan_id: number
  status: ScheduledTaskRunStatus
  trigger: 'schedule' | 'manual'
  scheduled_at: string
  started_at?: string | null
  finished_at?: string | null
  total: number
  completed: number
  running: number
  remaining: number
}

// 账号结果可包含重登操作编号、认证响应状态及测智的证据摘要。
export interface ScheduledTaskItem extends ScheduledTaskTarget {
  state: ScheduledTaskRunStatus
  stage?: string
  outcome?: string
  detail?: string
  probe?: {
    verdict: string
    mint_status: number
    continue_status: number
    reason?: string
  }
  answer?: string
  operation_id?: string
  replacement_id?: number
  http_status?: number
  auth_http_status?: number
  finished_at?: string | null
}

export interface ScheduledTaskRunDetail extends ScheduledTaskRun {
  items: ScheduledTaskItem[]
}

const base = '/admin/scheduled-tasks'

export async function listPlans(signal?: AbortSignal) {
  const { data } = await apiClient.get<{ items: ScheduledTaskPlan[]; timezone: string }>(`${base}/plans`, { signal })
  return data
}

export async function previewPlan(draft: ScheduledTaskDraft, signal?: AbortSignal) {
  const { data } = await apiClient.post<ScheduledTaskPreview>(`${base}/preview`, draft, { signal })
  return data
}

export async function savePlan(draft: ScheduledTaskDraft, id?: number) {
  const response = id === undefined
    ? await apiClient.post<ScheduledTaskPlan>(`${base}/plans`, draft)
    : await apiClient.put<ScheduledTaskPlan>(`${base}/plans/${id}`, draft)
  return response.data
}

export async function runPlan(id: number) {
  const { data } = await apiClient.post<{ run_id: number }>(`${base}/plans/${id}/run`)
  return data.run_id
}

export async function listRuns(id: number, signal?: AbortSignal) {
  const { data } = await apiClient.get<{ items: ScheduledTaskRun[] }>(`${base}/plans/${id}/runs`, { signal })
  return data.items
}

export async function readRun(id: number, signal?: AbortSignal) {
  const { data } = await apiClient.get<ScheduledTaskRunDetail>(`${base}/runs/${id}`, { signal })
  return data
}
