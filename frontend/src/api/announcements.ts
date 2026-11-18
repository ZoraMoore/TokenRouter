/**
 * User Announcements API endpoints
 */

import { apiClient } from './client'
import type { UserAnnouncement } from '@/types'

export async function list(
  unreadOnly: boolean = false,
  bypassCache: boolean = false,
): Promise<UserAnnouncement[]> {
  const params: Record<string, number> = {}
  if (unreadOnly) params.unread_only = 1

  // 主动刷新时在地址中加入时间戳，浏览器重新请求个性化公告。
  if (bypassCache) params.refresh_timestamp = Date.now()

  const { data } = await apiClient.get<UserAnnouncement[]>('/announcements', {
    params,
  })
  return data
}

export async function markRead(id: number): Promise<{ message: string }> {
  const { data } = await apiClient.post<{ message: string }>(`/announcements/${id}/read`)
  return data
}

const announcementsAPI = {
  list,
  markRead
}

export default announcementsAPI
