// 图形来自 Lucide；官方暂无对应动画，使用统一轻微缩放。
// https://github.com/lucide-icons/lucide/blob/66d8f9fc394b8530377e5f6112f0b8908ba01280/icons/circle-x.svg
import { createFallbackIcon } from '../createFallbackIcon'

export default createFallbackIcon('circle-x', [
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
      d: 'm15 9-6 6'
    }
  ],
  [
    'path',
    {
      d: 'm9 9 6 6'
    }
  ]
])
