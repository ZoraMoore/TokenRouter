// 图形来自 Lucide；官方暂无对应动画，使用统一轻微缩放。
// https://github.com/lucide-icons/lucide/blob/66d8f9fc394b8530377e5f6112f0b8908ba01280/icons/circle-user.svg
import { createFallbackIcon } from '../createFallbackIcon'

export default createFallbackIcon('circle-user', [
  [
    'circle',
    {
      cx: '12',
      cy: '12',
      r: '10'
    }
  ],
  [
    'circle',
    {
      cx: '12',
      cy: '10',
      r: '3'
    }
  ],
  [
    'path',
    {
      d: 'M7 20.662V19a2 2 0 0 1 2-2h6a2 2 0 0 1 2 2v1.662'
    }
  ]
])
