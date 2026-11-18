import { computed, reactive } from 'vue'

/**
 * useDirtyTracker 比较表单数据和上次保存时的快照，判断是否有未保存的修改。
 *
 * 每个数据源单独记录快照，各自加载完成后调用 markClean(key)。
 * 某个源记录快照时，其他源上已有的修改仍然算作未保存。
 * 还没记录快照的源算作未修改。快照用 JSON 序列化比较，数据源需要能被 JSON 表示。
 */
export function useDirtyTracker<K extends string>(sources: Record<K, () => unknown>) {
  const keys = Object.keys(sources) as K[]
  const baselines = reactive({}) as Partial<Record<K, string>>

  const serialize = (key: K) => JSON.stringify(sources[key]())

  // 不传 key 时为所有数据源记录快照。
  function markClean(...targets: K[]) {
    for (const key of targets.length > 0 ? targets : keys) {
      baselines[key] = serialize(key)
    }
  }

  // 某个数据源是否有未保存的修改，还没记录快照时返回 false。
  function isDirty(key: K) {
    return baselines[key] !== undefined && baselines[key] !== serialize(key)
  }

  const dirty = computed(() => keys.some(isDirty))

  return { dirty, isDirty, markClean }
}
