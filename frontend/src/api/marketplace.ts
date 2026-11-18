import { apiClient } from './client'
import type { MarketplaceGroup, MarketplaceStats } from '@/types'

export async function getMarketplaceModels(): Promise<MarketplaceGroup[]> {
  // 模型广场展示价格和可用率，跳过容量聚合可减少后端查询。
  const { data } = await apiClient.get<MarketplaceGroup[]>('/marketplace/models', {
    params: { include_capacity: false },
  })
  return data
}

export async function getMarketplaceStats(): Promise<MarketplaceStats> {
  const { data } = await apiClient.get<MarketplaceStats>('/marketplace/stats')
  return data
}

export const marketplaceAPI = {
  getMarketplaceModels,
  getMarketplaceStats,
}

export default marketplaceAPI
