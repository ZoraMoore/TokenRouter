import type { IconDefinition, IconTag } from './types'

/** 为暂无逐元素动画的 Lucide 图形提供 CSS 缩放反馈。 */
export function createFallbackIcon(
  name: string,
  nodes: Array<[IconTag, Record<string, string>]>
): IconDefinition {
  return {
    name,
    scale: true,
    render: (node) => nodes.map(([tag, attributes]) => node(tag, attributes))
  }
}
