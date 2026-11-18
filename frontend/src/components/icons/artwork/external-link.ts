// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/external-link.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const ARROW_VARIANTS: Variants = {
  normal: {
    scale: 1,
    translateX: 0,
    translateY: 0
  },
  animate: {
    scale: [1, 0.92, 1],
    translateX: [0, 2, 0],
    translateY: [0, -2, 0],
    originX: 1,
    originY: 0,
    transition: {
      duration: 0.5,
      ease: 'easeInOut'
    }
  }
}

const icon: IconDefinition = {
  name: 'external-link',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        'path',
        {
          d: 'M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6'
        },
        []
      ),
      h(
        motion.g,
        {
          animate: controls,
          variants: ARROW_VARIANTS,
          initial: 'normal'
        },
        () => [
          h(
            'path',
            {
              d: 'M15 3h6v6'
            },
            []
          ),
          h(
            'path',
            {
              d: 'M10 14 21 3'
            },
            []
          )
        ]
      )
    ])
}

export default icon
