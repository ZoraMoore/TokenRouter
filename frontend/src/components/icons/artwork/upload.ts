// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/upload.tsx
import type { IconDefinition, IconVariants as Variants } from '../types'

const ARROW_VARIANTS: Variants = {
  normal: { y: 0 },
  animate: {
    y: -2,
    transition: {
      type: 'spring',
      stiffness: 200,
      damping: 10,
      mass: 1
    }
  }
}

const icon: IconDefinition = {
  name: 'upload',
  normal: 'normal',
  animate: ['animate'],
  render: (node) =>
    node('g', {}, [
      node(
        'path',
        {
          d: 'M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4'
        },
        []
      ),
      node(
        'g',
        {
          animate: true,
          variants: ARROW_VARIANTS,
          initial: 'normal'
        },
        () => [
          node(
            'polyline',
            {
              points: '17 8 12 3 7 8'
            },
            []
          ),
          node(
            'line',
            {
              x1: '12',
              x2: '12',
              y1: '3',
              y2: '15'
            },
            []
          )
        ]
      )
    ])
}

export default icon
