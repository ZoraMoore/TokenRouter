import type { SelectOption } from '@/components/common/Select.vue'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'

// 平台筛选使用执行提供商的平台目录，分组选项独立提供。
export function buildOpsPlatformOptions(allLabel: string): SelectOption[] {
  return [{ value: '', label: allLabel }, ...CONCRETE_PLATFORM_OPTIONS]
}
