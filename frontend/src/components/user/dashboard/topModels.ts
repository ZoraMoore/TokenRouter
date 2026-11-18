import type { ModelStat } from '@/types'
import type { UsageMetric } from './usageChartState'

// RankedModel 是排行中的一个模型及其占比（0-100）。
export interface RankedModel {
  model: ModelStat
  share: number
}

// ModelRanking 是排行结果；others 汇总排行之外的模型，没有剩余模型时为 null。
export interface ModelRanking {
  top: RankedModel[]
  others: { count: number; amount: number; share: number } | null
}

// rankingAmountOf 取模型在排行指标下的数量；命中率不是可加总的量，按 Token 用量排序。
const rankingAmountOf = (model: ModelStat, metric: UsageMetric): number => {
  if (metric === 'requests') return model.requests
  if (metric === 'cost') return model.actual_cost
  return model.total_tokens
}

// rankModels 按当前指标降序取前 limit 个模型，并计算各自占全部模型合计的比例。
export const rankModels = (models: ModelStat[], metric: UsageMetric, limit: number): ModelRanking => {
  const sorted = models
    .filter((model) => rankingAmountOf(model, metric) > 0)
    .sort((a, b) => rankingAmountOf(b, metric) - rankingAmountOf(a, metric))
  const total = sorted.reduce((sum, model) => sum + rankingAmountOf(model, metric), 0)
  const shareOf = (amount: number) => (total > 0 ? (amount / total) * 100 : 0)
  const top = sorted.slice(0, limit).map((model) => ({ model, share: shareOf(rankingAmountOf(model, metric)) }))
  const rest = sorted.slice(limit)
  const restAmount = rest.reduce((sum, model) => sum + rankingAmountOf(model, metric), 0)
  return {
    top,
    others: rest.length > 0 ? { count: rest.length, amount: restAmount, share: shareOf(restAmount) } : null,
  }
}
