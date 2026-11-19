// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/search.tsx
import type { IconDefinition } from '../types'

const icon: IconDefinition = {
  name: 'search',
  normal: 'normal',
  animate: ['animate'],
  render: (node) =>
    node(
      'g',
      {
        animate: true,
        transition: {
          duration: 1,
          bounce: 0.3
        },
        variants: {
          normal: { x: 0, y: 0 },
          animate: {
            x: [0, 0, -3, 0],
            y: [0, -4, 0, 0]
          }
        },
        initial: 'normal'
      },
      () => [
        node(
          'circle',
          {
            cx: '11',
            cy: '11',
            r: '8'
          },
          []
        ),
        node(
          'path',
          {
            d: 'm21 21-4.3-4.3'
          },
          []
        )
      ]
    )
}

export default icon
