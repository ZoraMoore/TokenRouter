import { isIQQuality, isIQQuestion, isIQTrace, readIQStateProbe, type IQQualityResult, type IQQuestionResult, type IQStateProbeResult, type IQTraceResult } from '@/api/admin/providerIQ'

export type IQMode = 'question' | 'probe' | 'quality' | 'modeltrace'
export type IQRecord = { id: string; at: string; model: string; effort: string } & (
  | { mode: 'question'; samples: IQQuestionResult[] }
  | { mode: 'quality'; result: IQQualityResult }
  | { mode: 'probe'; result: IQStateProbeResult }
  | { mode: 'modeltrace'; result: IQTraceResult }
)

export interface IQSlot {
  state: 'running' | 'cancelled' | 'failed' | 'complete'
  result?: IQQuestionResult
  message?: string
}

const maxRecords = 12

function readRecord(value: unknown): IQRecord | null {
  if (!value || typeof value !== 'object') return null
  const data = value as Partial<IQRecord>
  if (typeof data.id !== 'string' || typeof data.at !== 'string' || typeof data.model !== 'string') return null
  const meta = { id: data.id, at: data.at, model: data.model, effort: typeof data.effort === 'string' ? data.effort : '' }
  if (data.mode === 'probe') {
    const result = readIQStateProbe(data.result)
    return result && { ...meta, mode: 'probe', result }
  }
  if (data.mode === 'quality' && isIQQuality(data.result)) return { ...meta, mode: 'quality', result: data.result }
  if (data.mode === 'modeltrace' && isIQTrace(data.result)) return { ...meta, mode: 'modeltrace', result: data.result }
  if (data.mode === 'question' && Array.isArray(data.samples) && data.samples.length > 0 && data.samples.every(isIQQuestion)) return { ...meta, mode: 'question', samples: data.samples }
  return null
}

export function iqHistoryKey(viewer: number, provider: number) {
  return `tokenrouter_provider_iq:${viewer}:${provider}`
}

export function readIQHistory(key: string): IQRecord[] {
  try {
    const saved: unknown = JSON.parse(localStorage.getItem(key) || '[]')
    if (!Array.isArray(saved)) return []
    return saved.map(readRecord).filter((item): item is IQRecord => item !== null).slice(0, maxRecords)
  } catch { return [] }
}

// 浏览器空间有限，较大的作答优先占用最早记录的空间。单条也放不下时，页面仍能查看本轮。
export function saveIQHistory(key: string, entry: IQRecord): { items: IQRecord[]; stored: boolean } {
  const items = [entry, ...readIQHistory(key).filter(item => item.id !== entry.id)].slice(0, maxRecords)
  for (let count = items.length; count > 0; count--) {
    const kept = items.slice(0, count)
    try {
      localStorage.setItem(key, JSON.stringify(kept))
      return { items: kept, stored: true }
    } catch { /* 尝试缩短浏览器保存的记录。 */ }
  }
  return { items, stored: false }
}
