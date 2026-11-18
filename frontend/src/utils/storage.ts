// readStorageWithLegacyKey 只在新键缺失时继承旧值，存储受限时仍可返回读到的数据。
export function readStorageWithLegacyKey(storage: Storage | null, key: string, legacyKey: string): string | null {
  if (!storage) return null
  try {
    const current = storage.getItem(key)
    if (current !== null) return current
    const legacy = storage.getItem(legacyKey)
    if (legacy !== null) {
      try {
        storage.setItem(key, legacy)
      } catch {
        // 浏览器配额或隐私策略可能禁止写入，旧值仍然有效。
      }
    }
    return legacy
  } catch {
    return null
  }
}
