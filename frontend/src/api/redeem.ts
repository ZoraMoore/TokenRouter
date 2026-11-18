/**
 * Redeem code API endpoints
 * Handles redeem code redemption for users
 */

import { apiClient } from './client'
import type { PaginatedResponse, RedeemCode, RedeemCodeRequest } from '@/types'

export type RedeemHistoryItem = RedeemCode

/**
 * Redeem a code
 * @param code - Redeem code string
 * @returns Redemption result with updated balance or concurrency
 */
export async function redeem(code: string): Promise<RedeemCode> {
  const payload: RedeemCodeRequest = { code }

  const { data } = await apiClient.post<RedeemCode>('/redeem', payload)

  return data
}

/**
 * 按使用时间倒序分页获取当前用户的兑换历史
 * @param page - 页码，从 1 开始
 * @param pageSize - 每页条数
 */
export async function getHistory(
  page: number,
  pageSize: number
): Promise<PaginatedResponse<RedeemHistoryItem>> {
  const { data } = await apiClient.get<PaginatedResponse<RedeemHistoryItem>>('/redeem/history', {
    params: { page, page_size: pageSize }
  })
  return data
}

export const redeemAPI = {
  redeem,
  getHistory
}

export default redeemAPI
