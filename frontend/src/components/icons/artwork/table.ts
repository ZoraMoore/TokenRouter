// 图形来自 Lucide；官方暂无对应动画，使用统一轻微缩放。
// https://github.com/lucide-icons/lucide/blob/66d8f9fc394b8530377e5f6112f0b8908ba01280/icons/table.svg
import { createFallbackIcon } from '../createFallbackIcon'

export default createFallbackIcon('table', [
  [
    'path',
    {
      d: 'M12 3v18'
    }
  ],
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
      d: 'M3 9h18'
    }
  ],
  [
    'path',
    {
      d: 'M3 15h18'
    }
  ]
])
