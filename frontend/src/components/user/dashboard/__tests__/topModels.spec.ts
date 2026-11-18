import { describe, expect, it } from 'vitest'

import type { ModelStat } from '@/types'
import { rankModels } from '../topModels'

const model = (name: string, requests: number, tokens: number, cost: number): ModelStat => ({
  model: name,
  requests,
  input_tokens: 0,
  output_tokens: 0,
  cache_creation_tokens: 0,
  cache_read_tokens: 0,
  total_tokens: tokens,
  cost,
  actual_cost: cost,
})

describe('rankModels', () => {
  const models = [model('a', 1, 600, 3), model('b', 3, 300, 1), model('c', 0, 100, 0), model('d', 2, 0, 0)]

  it('按指标降序排列并计算占全部模型的比例', () => {
    const ranking = rankModels(models, 'tokens', 2)
    expect(ranking.top.map((item) => item.model.model)).toEqual(['a', 'b'])
    expect(ranking.top.map((item) => item.share)).toEqual([60, 30])
    expect(ranking.others).toEqual({ count: 1, amount: 100, share: 10 })
  })

  it('跳过该指标为 0 的模型，命中率按 Token 排序', () => {
    expect(rankModels(models, 'requests', 5).top.map((item) => item.model.model)).toEqual(['b', 'd', 'a'])
    expect(rankModels(models, 'cacheHitRate', 5).top.map((item) => item.model.model)).toEqual(['a', 'b', 'c'])
  })

  it('全部模型都在排行内时没有其他行', () => {
    expect(rankModels(models, 'cost', 5).others).toBeNull()
    expect(rankModels([], 'cost', 5)).toEqual({ top: [], others: null })
  })
})
