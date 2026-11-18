import { apiClient } from './client'

export const pelicanQuestionKinds = ['pelican', 'candy', 'custom_svg', 'custom_text'] as const
export type PelicanQuestionKind = typeof pelicanQuestionKinds[number]
// 协议探针可单独排期，答题弹窗只使用上面的四种题型。
export const pelicanPlanKinds = [...pelicanQuestionKinds, 'state_probe'] as const
export type PelicanPlanKind = typeof pelicanPlanKinds[number]
export const pelicanEfforts = ['low', 'medium', 'high', 'xhigh'] as const
export type PelicanEffort = typeof pelicanEfforts[number]
export type PelicanReviewStatus = 'passed' | 'suspected'
export type PelicanStatus = 'pending_review' | PelicanReviewStatus | 'inconclusive'
export type PelicanRunState = 'idle' | 'queued' | 'running'

// 摘要保留作答时的强度。历史样本可能没有记录，default 表示这次请求使用了模型默认值。
export interface PelicanResult {
  id: number
  model_id: string
  question_kind: PelicanPlanKind
  question_name?: string
  reasoning_effort?: PelicanEffort | 'default' | 'none' | ''
  status: PelicanStatus
  tested_at: string
  latency_ms: number
  detail_code?: string
}

export interface PelicanResultDetail extends PelicanResult {
  prompt: string
  criteria: string[]
  answer: string
}

// 用户列表随糖果作答返回纯文本预览，详情接口提供完整作答。
export interface PelicanCandyAnswer extends PelicanResult {
  question_kind: 'candy'
  answer_preview: string
}

export interface PelicanModel {
  model_id: string
  question_kind: PelicanPlanKind
  question_name?: string
  reasoning_effort?: PelicanEffort | ''
  // 作品保留作答时的题型和模型，计划改题后仍可在原题下查看。
  artworks?: PelicanResult[]
  answers?: PelicanCandyAnswer[]
  enabled: boolean
  run_state: PelicanRunState
  next_run_at?: string | null
  latest: PelicanResult | null
}

export function isPelicanArtwork(kind: PelicanPlanKind): boolean {
  return kind === 'pelican' || kind === 'custom_svg'
}

export function isCustomQuestion(kind: PelicanPlanKind): boolean {
  return kind === 'custom_svg' || kind === 'custom_text'
}

export interface PelicanGroup {
  group_id: number
  group_name: string
  models: PelicanModel[]
}

export async function listPelicanGroups(signal: AbortSignal): Promise<PelicanGroup[]> {
  const { data } = await apiClient.get<PelicanGroup[]>('/pelican-tests', { signal })
  return data
}

export async function getPelicanResult(id: number, signal: AbortSignal): Promise<PelicanResultDetail> {
  const { data } = await apiClient.get<PelicanResultDetail>(`/pelican-tests/results/${id}`, { signal })
  return data
}
