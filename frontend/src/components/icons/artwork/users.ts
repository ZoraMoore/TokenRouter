// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/users.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const PATH_VARIANTS: Variants = {
  normal: {
    translateX: 0,
    transition: {
      type: 'spring',
      stiffness: 200,
      damping: 13
    }
  },
  animate: {
    translateX: [-6, 0],
    transition: {
      delay: 0.1,
      type: 'spring',
      stiffness: 200,
      damping: 13
    }
  }
}

const icon: IconDefinition = {
  name: 'users',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        'path',
        {
          d: 'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2'
        },
        []
      ),
      h(
        'circle',
        {
          cx: '9',
          cy: '7',
          r: '4'
        },
        []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M22 21v-2a4 4 0 0 0-3-3.87',
          variants: PATH_VARIANTS,
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M16 3.13a4 4 0 0 1 0 7.75',
          variants: PATH_VARIANTS,
          initial: 'normal'
        },
        () => []
      )
    ])
}

export default icon
