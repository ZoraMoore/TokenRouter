// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/ban.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const CIRCLE_VARIANTS: Variants = {
  normal: {
    opacity: 1,
    pathLength: 1,
    transition: {
      duration: 0.3,
      opacity: { duration: 0.1 }
    }
  },
  animate: {
    opacity: [0, 1],
    pathLength: [0, 1],
    transition: {
      duration: 0.4,
      opacity: { duration: 0.1 }
    }
  }
}

const LINE_VARIANTS: Variants = {
  normal: {
    opacity: 1,
    pathLength: 1,
    transition: {
      duration: 0.3,
      opacity: { duration: 0.1 }
    }
  },
  slash: () => ({
    opacity: [0, 1],
    pathLength: [0, 1],
    transition: {
      duration: 0.4,
      opacity: { duration: 0.1 }
    }
  })
}

const icon: IconDefinition = {
  name: 'ban',
  normal: 'normal',
  animate: ['animate', 'slash'],
  render: (controls) =>
    h('g', {}, [
      h(
        motion.circle,
        {
          animate: controls,
          cx: '12',
          cy: '12',
          initial: 'normal',
          r: '10',
          variants: CIRCLE_VARIANTS
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'm4.9 4.9 14.2 14.2',
          initial: 'normal',
          variants: LINE_VARIANTS
        },
        () => []
      )
    ])
}

export default icon
