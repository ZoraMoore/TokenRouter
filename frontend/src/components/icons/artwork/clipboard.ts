// 图形来自 Lucide；官方暂无对应动画，使用统一轻微缩放。
// https://github.com/lucide-icons/lucide/blob/66d8f9fc394b8530377e5f6112f0b8908ba01280/icons/clipboard.svg
import { createFallbackIcon } from '../createFallbackIcon'

export default createFallbackIcon('clipboard', [
  [
    'rect',
    {
      width: '8',
      height: '4',
      x: '8',
      y: '2',
      rx: '1',
      ry: '1'
    }
  ],
  [
    'path',
    {
      d: 'M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2'
    }
  ]
])
