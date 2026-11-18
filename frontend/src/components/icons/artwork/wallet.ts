// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/wallet.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const VARIANTS: Variants = {
  normal: {
    y: 0,
    rotate: 0,
    transition: {
      duration: 0.3,
      ease: 'easeOut'
    }
  },
  animate: {
    y: [0, -3, 0],
    rotate: [0, -4, 0],
    transition: {
      duration: 0.55,
      ease: 'easeInOut',
      times: [0, 0.45, 1]
    }
  }
}

const icon: IconDefinition = {
  name: 'wallet',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h(
      motion.g,
      {
        animate: controls,
        initial: 'normal',
        style: { transformOrigin: '12px 12px' },
        variants: VARIANTS
      },
      () => [
        h(
          'path',
          {
            d: 'M19 7V4a1 1 0 0 0-1-1H5a2 2 0 0 0 0 4h15a1 1 0 0 1 1 1v4h-3a2 2 0 0 0 0 4h3a1 1 0 0 0 1-1v-2a1 1 0 0 0-1-1'
          },
          []
        ),
        h(
          'path',
          {
            d: 'M3 5v14a2 2 0 0 0 2 2h15a1 1 0 0 0 1-1v-4'
          },
          []
        )
      ]
    )
}

export default icon
