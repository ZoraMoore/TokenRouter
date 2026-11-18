import { computed, inject, onBeforeUnmount, provide, shallowReactive, unref, type InjectionKey, type Ref } from 'vue'

/**
 * 一块可以单独保存的设置：dirty 表示有未保存的修改，save 返回是否保存成功，
 * discard 从服务器重新加载这块设置，放弃未保存的修改。
 */
export interface SettingsSaveTarget {
  dirty: Ref<boolean> | boolean
  save: () => Promise<boolean>
  discard: () => Promise<void>
}

interface SettingsSaveRegistry {
  register: (key: string, target: SettingsSaveTarget) => void
  unregister: (key: string) => void
}

const settingsSaveRegistryKey: InjectionKey<SettingsSaveRegistry> = Symbol('settings-save-registry')

/**
 * createSettingsSaveRegistry 汇总整页设置里各块设置的修改状态和保存函数，
 * 吸底保存条据此显示，并在保存时依次调用有修改的那几块。
 */
export function createSettingsSaveRegistry() {
  const targets = shallowReactive(new Map<string, SettingsSaveTarget>())

  const dirty = computed(() => [...targets.values()].some((target) => unref(target.dirty)))

  // 按登记顺序保存有修改的设置。某一块失败时继续保存其余几块，失败的那块保持未保存状态。
  async function saveDirty(): Promise<boolean> {
    let ok = true
    for (const target of [...targets.values()]) {
      if (!unref(target.dirty)) continue
      if (!(await target.save())) ok = false
    }
    return ok
  }

  // 有修改的几块同时从服务器重新加载。
  async function discardDirty() {
    await Promise.all(
      [...targets.values()].filter((target) => unref(target.dirty)).map((target) => target.discard()),
    )
  }

  const registry: SettingsSaveRegistry = {
    register: (key, target) => targets.set(key, target),
    unregister: (key) => targets.delete(key),
  }

  return { dirty, saveDirty, discardDirty, registry }
}

/** provideSettingsSaveRegistry 创建登记表并提供给子组件。 */
export function provideSettingsSaveRegistry() {
  const saveRegistry = createSettingsSaveRegistry()
  provide(settingsSaveRegistryKey, saveRegistry.registry)
  return saveRegistry
}

/** useSettingsSaveTarget 把当前组件的一块设置登记到上层的登记表，组件卸载时移除。 */
export function useSettingsSaveTarget(key: string, target: SettingsSaveTarget) {
  const registry = inject(settingsSaveRegistryKey, null)
  if (!registry) return
  registry.register(key, target)
  onBeforeUnmount(() => registry.unregister(key))
}

export { settingsSaveRegistryKey }
