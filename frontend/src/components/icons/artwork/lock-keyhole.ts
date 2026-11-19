// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/lock-keyhole.tsx
import type { IconDefinition } from '../types'

const icon: IconDefinition = {
  name: 'lock-keyhole',
  normal: 'normal',
  animate: ['animate'],
  render: (node) =>
    node(
      'g',
      {
        animate: true,
        initial: 'normal',
        transition: {
          duration: 1,
          ease: [0.4, 0, 0.2, 1]
        },
        variants: {
          normal: {
            rotate: 0,
            scale: 1
          },
          animate: {
            rotate: [-3, 1, -2, 0],
            scale: [0.95, 1.05, 0.98, 1]
          }
        }
      },
      () => [
        node(
          'circle',
          {
            cx: '12',
            cy: '16',
            r: '1'
          },
          []
        ),
        node(
          'rect',
          {
            height: '12',
            rx: '2',
            width: '18',
            x: '3',
            y: '10'
          },
          []
        ),
        node(
          'path',
          {
            animate: true,
            d: 'M7 10V7a5 5 0 0 1 10 0v3',
            initial: 'normal',
            transition: {
              duration: 0.3,
              ease: [0.4, 0, 0.2, 1]
            },
            variants: {
              normal: {
                pathLength: 1
              },
              animate: {
                pathLength: 0.7
              }
            }
          },
          () => []
        )
      ]
    )
}

export default icon
