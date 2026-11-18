// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/moon.tsx
import { h } from 'vue'
import { motion, type Transition } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const SVG_VARIANTS: Variants = {
  normal: {
    rotate: 0
  },
  animate: {
    rotate: [0, -10, 10, -5, 5, 0]
  }
}

const SVG_TRANSITION: Transition = {
  duration: 1.2,
  ease: 'easeInOut'
}

const icon: IconDefinition = {
  name: 'moon',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h(
      motion.g,
      {
        animate: controls,
        transition: SVG_TRANSITION,
        variants: SVG_VARIANTS,
        initial: 'normal'
      },
      () => [
        h(
          'path',
          {
            d: 'M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z'
          },
          []
        )
      ]
    )
}

export default icon
