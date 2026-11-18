// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/chevron-left.tsx
import { h } from 'vue'
import { motion, type Transition } from 'motion-v'
import type { IconDefinition } from '../types'

const DEFAULT_TRANSITION: Transition = {
  times: [0, 0.4, 1],
  duration: 0.5
}

const icon: IconDefinition = {
  name: 'chevron-left',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        motion.path,
        {
          animate: controls,
          d: 'm15 18-6-6 6-6',
          transition: DEFAULT_TRANSITION,
          variants: {
            normal: { x: 0 },
            animate: { x: [0, -2, 0] }
          },
          initial: 'normal'
        },
        () => []
      )
    ])
}

export default icon
