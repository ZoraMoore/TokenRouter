import { getConfiguredTableDefaultPageSize, normalizeTablePageSize } from '@/utils/tablePreferences'

/**
 * 读取当前系统配置的表格默认每页条数。
 * 各页面使用通用表格设置中的默认值。
 */
export function getPersistedPageSize(fallback = getConfiguredTableDefaultPageSize()): number {
  return normalizeTablePageSize(getConfiguredTableDefaultPageSize() || fallback)
}

export function setPersistedPageSize(_size: number): void {
  // 每页条数由运行时表格配置控制。
}
