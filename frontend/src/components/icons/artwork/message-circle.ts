// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/message-circle.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const ICON_VARIANTS: Variants = {
  normal: {
    scale: 1,
    rotate: 0
  },
  animate: {
    scale: 1.05,
    rotate: [0, -7, 7, 0],
    transition: {
      rotate: {
        duration: 0.5,
        ease: 'easeInOut'
      },
      scale: {
        type: 'spring',
        stiffness: 400,
        damping: 10
      }
    }
  }
}

const icon: IconDefinition = {
  name: 'message-circle',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h(
      motion.g,
      {
        animate: controls,
        variants: ICON_VARIANTS,
        initial: 'normal'
      },
      () => [
        h(
          'path',
          {
            d: 'M7.9 20A9 9 0 1 0 4 16.1L2 22Z'
          },
          []
        )
      ]
    )
}

export default icon
