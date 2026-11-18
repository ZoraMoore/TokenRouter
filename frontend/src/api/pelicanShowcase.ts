import { apiClient } from './client'

export interface PelicanVisibility {
  enabled: boolean
  api_enabled: boolean
}

export interface PelicanShowcaseSettings extends PelicanVisibility {
  max_items: number
  auto_cleanup: boolean
  retention_days: number
}

const settingsPath = '/admin/pelican-showcase/settings'

// 读取和提交都使用这五个字段，布尔开关和数字范围与设置表单一致。
function showcaseSettings(value: PelicanShowcaseSettings): PelicanShowcaseSettings {
  if (!value || typeof value.enabled !== 'boolean' || typeof value.api_enabled !== 'boolean' ||
    typeof value.auto_cleanup !== 'boolean' || !Number.isInteger(value.max_items) || value.max_items < 1 || value.max_items > 100 ||
    !Number.isInteger(value.retention_days) || value.retention_days < 1 || value.retention_days > 90) {
    throw new Error('用户展示设置格式有误')
  }
  const { enabled, api_enabled, auto_cleanup, max_items, retention_days } = value
  return { enabled, api_enabled, max_items, auto_cleanup, retention_days }
}

export async function getPelicanVisibility(signal: AbortSignal): Promise<PelicanVisibility> {
  const { data } = await apiClient.get<PelicanVisibility>('/pelican-tests/visibility', { signal })
  if (!data || typeof data.enabled !== 'boolean' || typeof data.api_enabled !== 'boolean') throw new Error('用户展示状态格式有误')
  return { enabled: data.enabled, api_enabled: data.api_enabled }
}

export async function getShowcaseSettings(signal: AbortSignal): Promise<PelicanShowcaseSettings> {
  const { data } = await apiClient.get<PelicanShowcaseSettings>(settingsPath, { signal })
  return showcaseSettings(data)
}

// 服务端按整份设置保存，关闭天数清理时仍保存天数，供下次开启使用。
export async function updateShowcaseSettings(input: PelicanShowcaseSettings): Promise<PelicanShowcaseSettings> {
  const { data } = await apiClient.put<PelicanShowcaseSettings>(settingsPath, showcaseSettings(input))
  return showcaseSettings(data)
}
