import { computed, ref, watch, type ComputedRef } from 'vue'
import type { Column } from '@/components/common/types'

// 选择和操作属于表格结构，始终留在列定义指定的位置。
const isStructuralColumn = (key: string) => key === 'select' || key === 'actions'

export function useTableColumnOrder(
  columns: () => Column[],
  storageKey: ComputedRef<string | undefined>
) {
  const columnOrder = ref<string[]>([])

  const readOrder = (key: string | undefined): string[] => {
    if (!key) return []
    try {
      const stored: unknown = JSON.parse(localStorage.getItem(key) || '[]')
      if (!Array.isArray(stored)) return []
      return [...new Set(stored.filter((value): value is string =>
        typeof value === 'string' && !isStructuralColumn(value)
      ))]
    } catch {
      return []
    }
  }

  watch(
    [storageKey, () => columns().map(column => column.key)],
    ([key, keys], previous) => {
      if (key !== previous?.[0]) columnOrder.value = readOrder(key)
      // 保留隐藏列的位置，新出现的列追加到数据列末尾。
      const known = new Set(columnOrder.value)
      columnOrder.value = [
        ...columnOrder.value,
        ...keys.filter(key => !isStructuralColumn(key) && !known.has(key))
      ]
    },
    { immediate: true }
  )

  const orderedColumns = computed(() => {
    if (!storageKey.value) return columns()
    const positions = new Map(columnOrder.value.map((key, index) => [key, index]))
    const movable = columns()
      .filter(column => !isStructuralColumn(column.key))
      .sort((a, b) => (positions.get(a.key) ?? Infinity) - (positions.get(b.key) ?? Infinity))
    let index = 0
    return columns().map(column => isStructuralColumn(column.key) ? column : movable[index++])
  })

  const movableColumns = computed(() => orderedColumns.value.filter(column => !isStructuralColumn(column.key)))
  const canReorder = (key: string) => Boolean(storageKey.value)
    && movableColumns.value.length > 1
    && movableColumns.value.some(column => column.key === key)

  const moveColumn = (source: string, target: string, side: 'before' | 'after') => {
    if (source === target || !canReorder(source) || !canReorder(target)) return false
    const next = columnOrder.value.filter(key => key !== source)
    const targetIndex = next.indexOf(target)
    next.splice(targetIndex + (side === 'after' ? 1 : 0), 0, source)
    columnOrder.value = next
    try {
      localStorage.setItem(storageKey.value!, JSON.stringify(next))
    } catch {
      // 浏览器禁用存储时，本次页面内仍可调整顺序。
    }
    return true
  }

  return { orderedColumns, movableColumns, canReorder, moveColumn }
}
