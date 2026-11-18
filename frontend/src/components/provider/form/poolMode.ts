export const DEFAULT_POOL_MODE_RETRY_COUNT = 3
export const MAX_POOL_MODE_RETRY_COUNT = 10
export const DEFAULT_POOL_MODE_RETRY_STATUS_CODES = [401, 403, 429]

// collectStatusCodes 去重并过滤非法 HTTP 状态码，结果升序排列。
function collectStatusCodes(values: Iterable<unknown>): number[] {
  const seen = new Set<number>()
  const out: number[] = []
  for (const value of values) {
    const n = typeof value === 'string' ? Number(value.trim()) : Number(value)
    if (!Number.isFinite(n) || !Number.isInteger(n)) continue
    if (n < 100 || n > 599) continue
    if (seen.has(n)) continue
    seen.add(n)
    out.push(n)
  }
  return out.sort((a, b) => a - b)
}

// parsePoolModeRetryStatusCodes 解析逗号或空白分隔的状态码输入。
export function parsePoolModeRetryStatusCodes(input: string): number[] {
  if (!input || !input.trim()) return []
  return collectStatusCodes(input.split(/[,\s]+/).filter(token => token.trim()))
}

// formatPoolModeRetryStatusCodes 把已保存的状态码数组还原为输入框文本。
export function formatPoolModeRetryStatusCodes(value: unknown): string {
  if (!Array.isArray(value)) return ''
  return collectStatusCodes(value).join(', ')
}

// normalizePoolModeRetryCount 将重试次数限制为 0 到上限之间的整数。
export function normalizePoolModeRetryCount(value: number): number {
  if (!Number.isFinite(value)) {
    return DEFAULT_POOL_MODE_RETRY_COUNT
  }
  const normalized = Math.trunc(value)
  if (normalized < 0) {
    return 0
  }
  if (normalized > MAX_POOL_MODE_RETRY_COUNT) {
    return MAX_POOL_MODE_RETRY_COUNT
  }
  return normalized
}
