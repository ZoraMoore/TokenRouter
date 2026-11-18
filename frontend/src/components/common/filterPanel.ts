import type { ComputedRef, InjectionKey } from 'vue'

// FilterControlState 是筛选字段内控件上报的当前状态，供面板生成已选条件标签。
export interface FilterControlState {
  active: boolean
  text: string
  clear: () => void
}

// FilterChip 是筛选面板顶部的一条已选条件，el 用于按字段在面板中的位置排序。
export interface FilterChip {
  label: string
  text: string
  clear: () => void
  el: HTMLElement | null
}

export interface FilterPanelContext {
  register: (chip: ComputedRef<FilterChip | null>) => () => void
}

export interface FilterFieldContext {
  // emptyValue 是字段未生效时的取值，undefined 表示由控件自行推断。
  emptyValue: () => unknown
  bindControl: (state: ComputedRef<FilterControlState>) => void
}

export const FILTER_PANEL_KEY: InjectionKey<FilterPanelContext> = Symbol('filterPanel')
export const FILTER_FIELD_KEY: InjectionKey<FilterFieldContext> = Symbol('filterField')

const isBlank = (value: unknown) => value === null || value === undefined || value === ''

// 首项为空值或 all 时视为“全部”，其他情况用 null 表示空选项。
export function resolveEmptyValue(firstOptionValue: unknown): unknown {
  return isBlank(firstOptionValue) || firstOptionValue === 'all' ? firstOptionValue : null
}

// null、undefined 和空串都表示未选择，彼此视为相同。
export function isSameFilterValue(value: unknown, emptyValue: unknown): boolean {
  return value === emptyValue || (isBlank(value) && isBlank(emptyValue))
}
