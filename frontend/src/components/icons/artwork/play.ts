// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/play.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const PATH_VARIANTS: Variants = {
  normal: {
    x: 0,
    rotate: 0
  },
  animate: {
    x: [0, -1, 2, 0],
    rotate: [0, -10, 0, 0],
    transition: {
      duration: 0.5,
      times: [0, 0.2, 0.5, 1],
      stiffness: 260,
      damping: 20
    }
  }
}

const icon: IconDefinition = {
  name: 'play',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h(
      motion.g,
      {
        initial: 'normal'
      },
      () => [
        h(
          motion.polygon,
          {
            animate: controls,
            points: '6 3 20 12 6 21 6 3',
            variants: PATH_VARIANTS,
            initial: 'normal'
          },
          () => []
        )
      ]
    )
}

export default icon
