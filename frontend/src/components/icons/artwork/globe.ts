// 图形来自 Lucide；官方暂无对应动画，使用统一轻微缩放。
// https://github.com/lucide-icons/lucide/blob/66d8f9fc394b8530377e5f6112f0b8908ba01280/icons/globe.svg
import { createFallbackIcon } from '../createFallbackIcon'

export default createFallbackIcon('globe', [
  [
    'circle',
    {
      cx: '12',
      cy: '12',
      r: '10'
    }
  ],
  [
    'path',
    {
      d: 'M12 2a14.5 14.5 0 0 0 0 20 14.5 14.5 0 0 0 0-20'
    }
  ],
  [
    'path',
    {
      d: 'M2 12h20'
    }
  ]
])
