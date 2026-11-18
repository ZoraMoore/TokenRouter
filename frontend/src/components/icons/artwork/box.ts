// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/box.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const PATH_VARIANTS: Variants = {
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

const icon: IconDefinition = {
  name: 'box',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        motion.path,
        {
          animate: controls,
          d: 'M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z',
          initial: 'normal',
          variants: PATH_VARIANTS
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'm3.3 7 8.7 5 8.7-5',
          initial: 'normal',
          variants: PATH_VARIANTS
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M12 22V12',
          initial: 'normal',
          variants: PATH_VARIANTS
        },
        () => []
      )
    ])
}

export default icon
