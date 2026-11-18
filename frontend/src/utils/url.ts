/**
 * 验证并规范化 URL
 * 默认只接受绝对 URL（以 http:// 或 https:// 开头），可按需允许相对路径
 * @param value 用户输入的 URL
 * @returns 规范化后的 URL，如果无效则返回空字符串
 */
type SanitizeOptions = {
  allowRelative?: boolean
  allowDataUrl?: boolean
}

export function sanitizeUrl(value: string, options: SanitizeOptions = {}): string {
  const trimmed = value.trim()
  if (!trimmed) {
    return ''
  }

  if (options.allowRelative && trimmed.startsWith('/') && !trimmed.startsWith('//')) {
    return trimmed
  }

  // 允许 data:image/ 开头的 data URL（仅限图片类型）
  if (options.allowDataUrl && trimmed.startsWith('data:image/')) {
    return trimmed
  }

  // 接受以 http:// 或 https:// 开头的绝对 URL。
  if (!trimmed.match(/^https?:\/\//i)) {
    return ''
  }

  try {
    const parsed = new URL(trimmed)
    const protocol = parsed.protocol.toLowerCase()
    if (protocol !== 'http:' && protocol !== 'https:') {
      return ''
    }
    return parsed.toString()
  } catch {
    return ''
  }
}
