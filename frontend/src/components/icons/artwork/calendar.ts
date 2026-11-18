// 图形来自 Lucide；官方暂无对应动画，使用统一轻微缩放。
// https://github.com/lucide-icons/lucide/blob/66d8f9fc394b8530377e5f6112f0b8908ba01280/icons/calendar.svg
import { createFallbackIcon } from '../createFallbackIcon'

export default createFallbackIcon('calendar', [
  [
    'path',
    {
      d: 'M8 2v3'
    }
  ],
  [
    'path',
    {
      d: 'M16 2v3'
    }
  ],
  [
    'rect',
    {
      x: '3',
      y: '3',
      width: '18',
      height: '18',
      rx: '2'
    }
  ],
  [
    'path',
    {
      d: 'M3 9h18'
    }
  ]
])
