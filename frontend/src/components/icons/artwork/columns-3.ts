// 图形来自 Lucide；官方暂无对应动画，使用统一轻微缩放。
// https://github.com/lucide-icons/lucide/blob/66d8f9fc394b8530377e5f6112f0b8908ba01280/icons/columns-3.svg
import { createFallbackIcon } from '../createFallbackIcon'

export default createFallbackIcon('columns-3', [
  [
    'rect',
    {
      width: '18',
      height: '18',
      x: '3',
      y: '3',
      rx: '2'
    }
  ],
  [
    'path',
    {
      d: 'M9 3v18'
    }
  ],
  [
    'path',
    {
      d: 'M15 3v18'
    }
  ]
])
