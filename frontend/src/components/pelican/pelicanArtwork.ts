import { isPelicanArtwork, pelicanEfforts, type PelicanCandyAnswer, type PelicanGroup, type PelicanModel, type PelicanPlanKind, type PelicanResult, type PelicanResultDetail } from '@/api/pelicanTests'
import { overriddenQuestionName } from '@/i18n/pelicanQuestionNames'
import { extractPelicanMarkup } from './pelicanMarkup'

export type PelicanArtworkBody = { html: string; image?: never } | { image: string; html?: never }

// 卡片和详情共用原文。早期记录只保存了可以独立显示的 SVG。
export function pelicanArtworkBody(sample: Pick<PelicanResultDetail, 'question_kind' | 'status' | 'raw_answer'> & { answer?: string }): PelicanArtworkBody | null {
  if (!isPelicanArtwork(sample.question_kind) || sample.status === 'inconclusive') return null
  if (sample.raw_answer) {
    const html = extractPelicanMarkup(sample.raw_answer)
    return html ? { html } : null
  }
  const image = pelicanImageSource(sample.answer || '')
  return image ? { image } : null
}

// 模型可能用 HTML 包住 SVG。取出 SVG 后，通过 img 的图片解码器显示。
export function pelicanImageSource(answer: string): string {
  const start = answer.search(/<svg(?=[\s>])/i)
  const end = answer.lastIndexOf('</svg')
  if (start < 0 || end < start) return ''
  const closing = /^<\/svg\s*>/.exec(answer.slice(end))
  if (!closing) return ''
  let svg = answer.slice(start, end + closing[0].length)
  const opening = svg.slice(0, svg.indexOf('>') + 1)
  if (!/\sxmlns\s*=/.test(opening)) svg = svg.replace(/^<svg/i, '<svg xmlns="http://www.w3.org/2000/svg"')
  try {
    return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(svg)
  } catch {
    return ''
  }
}

const recordedEfforts = new Set<string>([...pelicanEfforts, 'default', 'none'])

export function effortLabel(effort: PelicanResult['reasoning_effort']): string {
  return 'pelicanTests.effort.' + (effort && recordedEfforts.has(effort) ? effort : 'unknown')
}

// 短于 0.1 秒的采样显示原始毫秒数，其余耗时保留一位小数。
export function durationLabel(t: (key: string, params: Record<string, unknown>) => string, ms: number): string {
  if (ms < 100) return t('pelicanTests.durationMillis', { ms })
  return t('pelicanTests.durationSeconds', { seconds: (ms / 1000).toFixed(1) })
}

interface QuestionRow {
  group: PelicanGroup
  models: PelicanModel[]
  samples: (PelicanResult | PelicanCandyAnswer)[]
}

interface QuestionSection {
  key: string
  kind: PelicanPlanKind
  name?: string
  drawing: boolean
  rows: QuestionRow[]
}

// 计划按当前题目展示状态。作品按作答时的题名归组，以便管理员改题后仍能找到旧画。
export function pelicanQuestionSections(groups: PelicanGroup[]): QuestionSection[] {
  const questions = new Map<string, QuestionSection>()
  for (const group of groups) {
    const groupRows = new Map<string, QuestionRow>()
    function rowFor(source: Pick<PelicanModel, 'question_kind' | 'question_name'>): QuestionRow {
      const kind = source.question_kind
      const name = overriddenQuestionName(source)
      const key = JSON.stringify([kind, name || ''])
      const existing = groupRows.get(key)
      if (existing) return existing
      let question = questions.get(key)
      if (!question) {
        question = { key, kind, name, drawing: isPelicanArtwork(kind), rows: [] }
        questions.set(key, question)
      }
      const row = { group, models: [], samples: [] }
      groupRows.set(key, row)
      question.rows.push(row)
      return row
    }
    for (const model of group.models) {
      const row = rowFor(model)
      row.models.push(model)
      if (model.question_kind === 'candy' && model.answers) {
        row.samples.push(...model.answers)
      }
      // 清单省略作品集时，使用最近一次可预览的绘图作答。
      const latest = model.latest
      const artworks = model.artworks ?? (latest && latest.status !== 'inconclusive' ? [latest] : [])
      for (const artwork of artworks) {
        if (isPelicanArtwork(artwork.question_kind)) rowFor(artwork).samples.push(artwork)
      }
    }
    for (const row of groupRows.values()) {
      row.samples.sort(newestSampleFirst)
    }
  }
  return Array.from(questions.values()).sort((a, b) => Number(b.drawing) - Number(a.drawing))
}

function newestSampleFirst(a: PelicanResult, b: PelicanResult): number {
  const millis = Date.parse(b.tested_at) - Date.parse(a.tested_at)
  if (millis) return millis
  // Date.parse 只保留毫秒，数据库中同一毫秒内的作答还可能相差几微秒。
  const fraction = (time: string) => Number(time.match(/\.\d+/)?.[0] || 0)
  return fraction(b.tested_at) - fraction(a.tested_at) || b.id - a.id
}
