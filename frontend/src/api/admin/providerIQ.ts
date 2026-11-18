import { apiClient, buildApiUrl } from '@/api/client'
import { ADMIN_UI_REQUEST_HEADER } from '@/api/adminUIRequest'
import type { PelicanQuestionKind, PelicanStatus } from '@/api/pelicanTests'
import type { Provider } from '@/types'
import { isValidOpenAIModelID } from '@/utils/modelMappingRules'

export interface IQModelInput {
  model_id: string
  reasoning_effort: string
}

export interface IQQuestionInput extends IQModelInput {
  question_kind: Exclude<PelicanQuestionKind, 'state_probe'>
  prompt: string
  expected_answer?: string
  round_id: string
}

export interface IQQuestionResult {
  question_kind: string
  round_id: string
  requested_model: string
  mapped_model?: string
  protocol?: string
  reasoning_effort?: string
  prompt: string
  criteria?: string[]
  answer?: string
  status: PelicanStatus
  detail_code?: string
  started_at: string
  latency_ms: number
}

export interface IQQualityResult {
  requested_model: string
  reasoning_effort?: string
  verdict: 'passed' | 'mismatch' | 'inconclusive'
  passed: number
  total: number
  samples: IQQuestionResult[]
  started_at: string
  latency_ms: number
}

export const stateProbeDefaultModel = 'gpt-6-astra'
// 手动探针和定时探针共用同一套状态标签。
export const stateProbeStatus = { healthy: 'passed', degraded: 'suspected', inconclusive: 'inconclusive' } as const
const probeFailures = ['account_error', 'rate_limited', 'model_unsupported', 'upstream_error', 'network_error', 'stream_error', 'no_ticket', 'timeout', 'output_limit', 'invalid_state', 'unknown'] as const
type ProbeFailure = typeof probeFailures[number]

export interface IQStateProbeResult {
  account_id: number
  requested_model: string
  model: string
  reported_model?: string
  verdict: 'healthy' | 'degraded' | 'inconclusive'
  failure?: ProbeFailure
  mint_status: number
  continue_status: number
  mint_completed?: boolean
  continue_completed?: boolean
  mint_latency_ms?: number
  continue_latency_ms?: number
  mint_reported_model?: string
  continue_reported_model?: string
  new_ticket: boolean
  finished_at: string
  latency_ms: number
}

export function isSpecialOpenAIAuth(provider: Pick<Provider, 'credentials'>): boolean {
  const authMode = String(provider.credentials?.auth_mode ?? '').trim().toLowerCase()
  const legacyMode = String(provider.credentials?.openai_auth_mode ?? '').trim().toLowerCase()
  const pat = ['personalaccesstoken', 'personal_access_token']
  return authMode === 'agentidentity' || pat.includes(authMode) || pat.includes(legacyMode)
}

// 未配置协议字段的 OpenAI OAuth 账号默认启用 Responses，空数组表示已停用。
export function isStateProbeProvider(provider: Pick<Provider, 'platform' | 'type' | 'credentials' | 'parent_provider_id'> | null | undefined): boolean {
  if (!provider || provider.platform !== 'openai' || provider.type !== 'oauth' || provider.parent_provider_id != null || isSpecialOpenAIAuth(provider)) return false
  const protocols = provider.credentials?.upstream_protocols
  return protocols === undefined || Array.isArray(protocols) && protocols.includes('openai_responses')
}

// 模型别名先按精确映射查找，再匹配最长通配前缀。
function mappedIQModel(provider: Pick<Provider, 'credentials'>, requested: string): string {
  const mapping = provider.credentials?.model_mapping
  if (!mapping || typeof mapping !== 'object') return requested
  const entries = Object.entries(mapping).filter((entry): entry is [string, string] => typeof entry[1] === 'string')
  const exact = entries.find(([source]) => source === requested)
  if (exact) return exact[1]
  return entries.filter(([source]) => source.endsWith('*') && requested.startsWith(source.slice(0, -1)))
    .sort(([a], [b]) => b.length - a.length)[0]?.[1] ?? requested
}

const nonTextPrefixes = [
  'gpt-image', 'chatgpt-image', 'dall-e', 'sora-', 'tts-', 'whisper-', 'text-embedding', 'text-moderation', 'omni-moderation',
  'gpt-audio', 'gpt-realtime', 'gpt-4o-audio', 'gpt-4o-realtime', 'gpt-4o-transcribe',
  'gpt-4o-mini-audio', 'gpt-4o-mini-realtime', 'gpt-4o-mini-transcribe', 'gpt-4o-mini-tts',
]

function textModelName(mapped: string): boolean {
  const name = mapped.slice(mapped.lastIndexOf('/') + 1).toLowerCase()
  return !!name && !nonTextPrefixes.some(prefix => name.startsWith(prefix))
}

export function isIQTextModel(provider: Pick<Provider, 'credentials'>, model: string): boolean {
  return textModelName(mappedIQModel(provider, model.trim()))
}

export const validStateProbeModelName = isValidOpenAIModelID

export function isStateProbeTextModel(provider: Pick<Provider, 'credentials'>, model: string): boolean {
  const requested = model.trim()
  const mapped = mappedIQModel(provider, requested)
  return validStateProbeModelName(requested) && validStateProbeModelName(mapped) && textModelName(mapped)
}

interface TraceChallenge { id: string; prompt: string; expected_count: number }
interface TraceCandidate { model: string; family: string; probability: number }
interface TraceAnalysis {
  prediction: string
  probability: number
  used_outputs: number
  candidates: TraceCandidate[]
  families: TraceCandidate[]
}
interface TraceBank {
  source_commit?: string
  sha256: string
  algorithm: string
  models: string[]
}
export interface IQTraceSample {
  challenge: TraceChallenge
  text?: string
  mapped_model?: string
  reasoning_effort?: string
  environment?: { protocol?: string }
  parsed_numbers: number
  minimum_numbers: number
  accepted: boolean
  error?: string
  analysis?: TraceAnalysis
  latency_ms?: number
}
export interface IQTraceResult {
  requested_model: string
  reasoning_effort?: string
  bank: TraceBank
  samples: IQTraceSample[]
  analysis?: TraceAnalysis
  status: 'complete' | 'partial' | 'insufficient'
  disagreement: boolean
  created_at: string
}
export type IQTraceEvent =
  | { type: 'modeltrace_start'; data: { challenges: TraceChallenge[]; bank: TraceBank } }
  | { type: 'modeltrace_sample'; data: IQTraceSample }
  | { type: 'modeltrace_complete'; data: IQTraceResult }

// 题目和质量检测使用服务端执行时限，管理员可以在请求结束前取消。
export async function runIQQuestion(providerId: number, input: IQQuestionInput, signal: AbortSignal) {
  const { data } = await apiClient.post<IQQuestionResult>(`/admin/providers/${providerId}/iq-test`, input, { signal, timeout: 0 })
  if (!isIQQuestion(data)) throw new Error('IQ_INVALID_RESULT')
  return data
}

export async function runIQQuality(providerId: number, input: IQModelInput, signal: AbortSignal) {
  const { data } = await apiClient.post<IQQualityResult>(`/admin/providers/${providerId}/iq-test/quality`, input, { signal, timeout: 0 })
  if (!isIQQuality(data)) throw new Error('IQ_INVALID_RESULT')
  return data
}

export async function runIQStateProbe(providerId: number, input: Pick<IQModelInput, 'model_id'>, signal: AbortSignal) {
  const { data } = await apiClient.post<unknown>(`/admin/providers/${providerId}/state-probe`, { model_id: input.model_id }, { signal, timeout: 0 })
  const result = readIQStateProbe(data)
  if (!result || result.account_id !== providerId) throw new Error('IQ_INVALID_RESULT')
  return result
}

function record(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === 'object'
}

// 历史和导出使用同一份摘要，逐项读取可展示的字段。
export function readIQStateProbe(value: unknown): IQStateProbeResult | null {
  if (!record(value)) return null
  const { account_id, requested_model, model, reported_model, verdict, failure, mint_status, continue_status, new_ticket, finished_at, latency_ms,
    mint_completed, continue_completed, mint_latency_ms, continue_latency_ms, mint_reported_model, continue_reported_model } = value
  const httpStatus = (n: unknown): n is number => typeof n === 'number' && Number.isInteger(n) && (n === 0 || n >= 100 && n <= 599)
  const modelName = (n: unknown): n is string => typeof n === 'string' && (!n || validStateProbeModelName(n))
  const elapsed = (n: unknown): n is number => typeof n === 'number' && Number.isSafeInteger(n) && n >= 0
  if (typeof account_id !== 'number' || !Number.isSafeInteger(account_id) || account_id <= 0 ||
    !modelName(requested_model) || !modelName(model) || reported_model != null && !modelName(reported_model) ||
    mint_reported_model != null && !modelName(mint_reported_model) || continue_reported_model != null && !modelName(continue_reported_model) ||
    verdict !== 'healthy' && verdict !== 'degraded' && verdict !== 'inconclusive' ||
    !httpStatus(mint_status) || !httpStatus(continue_status) || typeof new_ticket !== 'boolean' ||
    typeof finished_at !== 'string' || !Number.isFinite(Date.parse(finished_at)) ||
    mint_completed != null && typeof mint_completed !== 'boolean' || continue_completed != null && typeof continue_completed !== 'boolean' ||
    !elapsed(latency_ms) || mint_latency_ms != null && !elapsed(mint_latency_ms) || continue_latency_ms != null && !elapsed(continue_latency_ms)) return null
  const failureCode = probeFailures.find(code => code === failure) || 'unknown'
  return { account_id, requested_model, model, reported_model: reported_model || undefined, verdict,
    failure: failure ? failureCode : undefined, mint_status, continue_status, new_ticket, finished_at, latency_ms,
    mint_completed: mint_completed ?? undefined, continue_completed: continue_completed ?? undefined,
    mint_latency_ms: mint_latency_ms ?? undefined, continue_latency_ms: continue_latency_ms ?? undefined,
    mint_reported_model: mint_reported_model || undefined, continue_reported_model: continue_reported_model || undefined }
}

export function isIQQuestion(value: unknown): value is IQQuestionResult {
  return record(value) && typeof value.question_kind === 'string' && typeof value.started_at === 'string' &&
    typeof value.requested_model === 'string' && typeof value.prompt === 'string' &&
    typeof value.latency_ms === 'number' && (value.answer == null || typeof value.answer === 'string') &&
    (value.criteria == null || Array.isArray(value.criteria) && value.criteria.every(item => typeof item === 'string')) &&
    ['passed', 'suspected', 'pending_review', 'inconclusive'].includes(String(value.status))
}

export function isIQQuality(value: unknown): value is IQQualityResult {
  return record(value) && ['passed', 'mismatch', 'inconclusive'].includes(String(value.verdict)) &&
    typeof value.passed === 'number' && typeof value.total === 'number' && typeof value.latency_ms === 'number' &&
    typeof value.requested_model === 'string' && typeof value.started_at === 'string' &&
    Array.isArray(value.samples) && value.samples.every(isIQQuestion)
}

function isChallenge(value: unknown): value is TraceChallenge {
  return record(value) && typeof value.id === 'string' && typeof value.prompt === 'string' && typeof value.expected_count === 'number'
}

function isBank(value: unknown): value is TraceBank {
  return record(value) && typeof value.sha256 === 'string' && typeof value.algorithm === 'string' &&
    Array.isArray(value.models) && value.models.every(model => typeof model === 'string')
}

function isAnalysis(value: unknown): value is TraceAnalysis {
  if (!record(value) || typeof value.prediction !== 'string' || typeof value.probability !== 'number' || typeof value.used_outputs !== 'number') return false
  const isCandidate = (item: unknown) => record(item) && typeof item.model === 'string' && typeof item.family === 'string' &&
    typeof item.probability === 'number' && item.probability >= 0 && item.probability <= 1
  return Array.isArray(value.candidates) && value.candidates.every(isCandidate) && Array.isArray(value.families) && value.families.every(isCandidate)
}

function isTraceSample(value: unknown): value is IQTraceSample {
  return record(value) && isChallenge(value.challenge) &&
    typeof value.parsed_numbers === 'number' && typeof value.minimum_numbers === 'number' &&
    typeof value.accepted === 'boolean' && (value.text == null || typeof value.text === 'string') &&
    (value.analysis == null || isAnalysis(value.analysis))
}

export function isIQTrace(value: unknown): value is IQTraceResult {
  return record(value) && typeof value.requested_model === 'string' && typeof value.created_at === 'string' &&
    isBank(value.bank) && (value.analysis == null || isAnalysis(value.analysis)) &&
    ['complete', 'partial', 'insufficient'].includes(String(value.status)) &&
    Array.isArray(value.samples) && value.samples.every(isTraceSample)
}

function decodeTraceEvent(raw: string): IQTraceEvent | undefined {
  let event: unknown
  try { event = JSON.parse(raw) } catch { throw new Error('IQ_INVALID_EVENT') }
  if (!record(event)) throw new Error('IQ_INVALID_EVENT')
  const data = event.data
  if (event.type === 'modeltrace_start' && record(data) && Array.isArray(data.challenges) && data.challenges.every(isChallenge) && isBank(data.bank)) {
    return { type: event.type, data: { challenges: data.challenges, bank: data.bank } }
  }
  if (event.type === 'modeltrace_sample' && isTraceSample(data)) return { type: event.type, data }
  if (event.type === 'modeltrace_complete' && isIQTrace(data)) return { type: event.type, data }
  if (event.type === 'error' || String(event.type).startsWith('modeltrace_')) throw new Error('IQ_INVALID_EVENT')
}

// 心跳和单题进度都可能出现在连接中断前，指纹历史在收到完成事件后才可写入。
export async function runIQTrace(providerId: number, input: IQModelInput, signal: AbortSignal, onEvent: (event: IQTraceEvent) => void): Promise<IQTraceResult> {
  const headers: Record<string, string> = { 'Content-Type': 'application/json', Accept: 'text/event-stream', [ADMIN_UI_REQUEST_HEADER]: '1' }
  const token = localStorage.getItem('auth_token')
  if (token) headers.Authorization = `Bearer ${token}`
  const response = await fetch(buildApiUrl(`/admin/providers/${providerId}/iq-test/modeltrace`), {
    method: 'POST', headers, body: JSON.stringify(input), signal, credentials: 'include',
  })
  if (!response.ok) {
    const body: unknown = response.headers.get('Content-Type')?.includes('application/json') ? await response.json().catch(() => null) : null
    const reason = record(body) && body.reason === 'IQ_TEST_UNSUPPORTED' ? body.reason : undefined
    throw Object.assign(new Error('IQ_REQUEST_FAILED'), { status: response.status, reason })
  }
  if (!response.body) throw new Error('IQ_MISSING_STREAM')
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  try {
    while (true) {
      const { value, done } = await reader.read()
      signal.throwIfAborted()
      buffer += decoder.decode(value, { stream: !done })
      let separator = /\r?\n\r?\n/.exec(buffer)
      while (separator) {
        const data = buffer.slice(0, separator.index).split(/\r?\n/).filter(line => line.startsWith('data:'))
          .map(line => line.slice(5).trimStart()).join('\n')
        buffer = buffer.slice(separator.index + separator[0].length)
        if (data) {
          const event = decodeTraceEvent(data)
          if (event) {
            onEvent(event)
            if (event.type === 'modeltrace_complete') return event.data
          }
        }
        separator = /\r?\n\r?\n/.exec(buffer)
      }
      if (done || buffer.length > 4 * 1024 * 1024) throw new Error('IQ_INCOMPLETE_STREAM')
    }
  } finally {
    await reader.cancel().catch(() => undefined)
    reader.releaseLock()
  }
}
