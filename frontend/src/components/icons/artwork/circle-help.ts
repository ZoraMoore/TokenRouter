// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/circle-help.tsx
import type { IconDefinition, IconVariants as Variants } from '../types'

const VARIANTS: Variants = {
  normal: { rotate: 0 },
  animate: { rotate: [0, -10, 10, -10, 0] }
}

const icon: IconDefinition = {
  name: 'circle-help',
  normal: 'normal',
  animate: ['animate'],
  render: (node) =>
    node('g', {}, [
      node(
        'circle',
        {
          cx: '12',
          cy: '12',
          r: '10'
        },
        []
      ),
      node(
        'g',
        {
          animate: true,
          transition: {
            duration: 0.5,
            ease: 'easeInOut'
          },
          variants: VARIANTS,
          initial: 'normal'
        },
        () => [
          node(
            'path',
            {
              d: 'M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3'
            },
            []
          ),
          node(
            'path',
            {
              d: 'M12 17h.01'
            },
            []
          )
        ]
      )
    ])
}

export default icon
