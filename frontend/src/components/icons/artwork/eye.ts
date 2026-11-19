// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/eye.tsx
import type { IconDefinition } from '../types'

const icon: IconDefinition = {
  name: 'eye',
  normal: 'normal',
  animate: ['animate'],
  render: (node) =>
    node('g', {}, [
      node(
        'path',
        {
          animate: true,
          d: 'M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0',
          style: { originY: '50%' },
          transition: { duration: 0.4, ease: 'easeInOut' },
          variants: {
            normal: { scaleY: 1, opacity: 1 },
            animate: { scaleY: [1, 0.1, 1], opacity: [1, 0.3, 1] }
          },
          initial: 'normal'
        },
        () => []
      ),
      node(
        'circle',
        {
          animate: true,
          cx: '12',
          cy: '12',
          r: '3',
          transition: { duration: 0.4, ease: 'easeInOut' },
          variants: {
            normal: { scale: 1, opacity: 1 },
            animate: { scale: [1, 0.3, 1], opacity: [1, 0.3, 1] }
          },
          initial: 'normal'
        },
        () => []
      )
    ])
}

export default icon
