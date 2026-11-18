// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/circle-help.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const VARIANTS: Variants = {
  normal: { rotate: 0 },
  animate: { rotate: [0, -10, 10, -10, 0] }
}

const icon: IconDefinition = {
  name: 'circle-help',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        'circle',
        {
          cx: '12',
          cy: '12',
          r: '10'
        },
        []
      ),
      h(
        motion.g,
        {
          animate: controls,
          transition: {
            duration: 0.5,
            ease: 'easeInOut'
          },
          variants: VARIANTS,
          initial: 'normal'
        },
        () => [
          h(
            'path',
            {
              d: 'M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3'
            },
            []
          ),
          h(
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
