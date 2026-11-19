import { apiClient } from '../client'
import { isPelicanArtwork, type PelicanEffort, type PelicanPlanKind, type PelicanResult, type PelicanResultDetail, type PelicanReviewStatus, type PelicanRunState } from '../pelicanTests'

const base = '/admin/pelican-tests'

export interface PelicanPlanInput {
  group_id: number
  provider_id: number | null
  model_id: string
  question_kind: PelicanPlanKind
  question_name?: string
  question_prompt?: string
  expected_answer?: string
  reasoning_effort?: PelicanEffort | ''
  cron_expression: string
  enabled: boolean
  suspend_on_wrong_answer?: boolean
}

// 从账号的智商测试建立计划时，可带入模型和题型。
export type PelicanPlanTarget = Pick<PelicanPlanInput, 'model_id' | 'question_kind' | 'reasoning_effort'> & { provider_id: number }

export interface PelicanPlan extends PelicanPlanInput {
  id: number
  group_name: string
  provider_name?: string
  run_state: PelicanRunState
  last_run_at?: string | null
  next_run_at?: string | null
}

export interface PelicanProvider {
  id: number
  name: string
  supports_state_probe: boolean
}

export interface PelicanQuestionDefault {
  question_kind: 'pelican' | 'candy'
  question_name: string
  question_prompt: string
  expected_answer?: string
}

// 运行记录里的提供商是本次实际取样的提供商，计划可以配置为自动调度。
export interface AdminPelicanResult extends PelicanResult {
  provider_id?: number | null
  provider_name?: string
  round_id?: string
}

export type AdminPelicanResultDetail = AdminPelicanResult & PelicanResultDetail

// 切换定时开关时也要提交完整计划。题型决定哪些题面和答案字段可以发送。
function planPayload(plan: PelicanPlanInput): PelicanPlanInput {
  const { group_id, provider_id, model_id, question_kind, cron_expression, enabled } = plan
  const input: PelicanPlanInput = { group_id, provider_id, model_id, question_kind, cron_expression, enabled }
  if (plan.question_name?.trim()) input.question_name = plan.question_name.trim()
  if (question_kind === 'state_probe') return input
  if (plan.question_prompt?.trim()) input.question_prompt = plan.question_prompt.trim()
  if (!isPelicanArtwork(question_kind) && plan.expected_answer?.trim()) input.expected_answer = plan.expected_answer.trim()
  if (plan.reasoning_effort) input.reasoning_effort = plan.reasoning_effort
  if (question_kind === 'candy' && provider_id === null && plan.suspend_on_wrong_answer) input.suspend_on_wrong_answer = true
  return input
}

// 两种内置题由服务端提供题面，糖果题同时提供判分用的参考答案。
export async function listQuestionDefaults(signal: AbortSignal): Promise<PelicanQuestionDefault[]> {
  const { data } = await apiClient.get<PelicanQuestionDefault[]>(`${base}/question-defaults`, { signal })
  if (!Array.isArray(data) || !['pelican', 'candy'].every(kind => data.some(question =>
    question?.question_kind === kind && typeof question.question_name === 'string' && question.question_name.trim() &&
    typeof question.question_prompt === 'string' && question.question_prompt.trim() &&
    (kind === 'pelican' || typeof question.expected_answer === 'string' && question.expected_answer.trim()),
  ))) throw new Error('内置题目缺少名称、题面或参考答案')
  return data
}

export async function listPlans(signal: AbortSignal): Promise<PelicanPlan[]> {
  const { data } = await apiClient.get<PelicanPlan[]>(`${base}/plans`, { signal })
  return data
}

export async function listProviders(groupId: number, signal: AbortSignal): Promise<PelicanProvider[]> {
  const { data } = await apiClient.get<PelicanProvider[]>(`${base}/groups/${groupId}/providers`, { signal })
  return data
}

export async function createPlan(plan: PelicanPlanInput): Promise<void> {
  await apiClient.post(`${base}/plans`, planPayload(plan))
}

export async function updatePlan(id: number, plan: PelicanPlanInput): Promise<PelicanPlan> {
  const { data } = await apiClient.put<PelicanPlan>(`${base}/plans/${id}`, planPayload(plan))
  return data
}

export async function deletePlan(id: number): Promise<void> {
  await apiClient.delete(`${base}/plans/${id}`)
}

export async function runPlan(id: number): Promise<void> {
  await apiClient.post(`${base}/plans/${id}/run`)
}

export async function listResults(planId: number, signal: AbortSignal): Promise<AdminPelicanResult[]> {
  const { data } = await apiClient.get<AdminPelicanResult[]>(`${base}/plans/${planId}/results`, { signal })
  return data
}

export async function getResult(id: number, signal: AbortSignal): Promise<AdminPelicanResultDetail> {
  const { data } = await apiClient.get<AdminPelicanResultDetail>(`${base}/results/${id}`, { signal })
  return data
}

export async function reviewResult(id: number, status: PelicanReviewStatus): Promise<void> {
  await apiClient.put(`${base}/results/${id}/review`, { status })
}
