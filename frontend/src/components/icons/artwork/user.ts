// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/user.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const PATH_VARIANT: Variants = {
  normal: { pathLength: 1, opacity: 1, pathOffset: 0 },
  animate: {
    pathLength: [0, 1],
    opacity: [0, 1],
    pathOffset: [1, 0]
  }
}

const CIRCLE_VARIANT: Variants = {
  normal: {
    pathLength: 1,
    pathOffset: 0,
    scale: 1
  },
  animate: {
    pathLength: [0, 1],
    pathOffset: [1, 0],
    scale: [0.5, 1]
  }
}

const icon: IconDefinition = {
  name: 'user',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        motion.circle,
        {
          animate: controls,
          cx: '12',
          cy: '8',
          r: '5',
          variants: CIRCLE_VARIANT,
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M20 21a8 8 0 0 0-16 0',
          transition: {
            delay: 0.2,
            duration: 0.4
          },
          variants: PATH_VARIANT,
          initial: 'normal'
        },
        () => []
      )
    ])
}

export default icon
