// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/layers.tsx
import { h } from 'vue'
import { motion, type Transition } from 'motion-v'
import type { IconDefinition } from '../types'

const DEFAULT_TRANSITION: Transition = {
  type: 'spring',
  stiffness: 100,
  damping: 14,
  mass: 1
}

const icon: IconDefinition = {
  name: 'layers',
  normal: 'normal',
  animate: ['firstState', 'secondState'],
  render: (controls) =>
    h('g', {}, [
      h(
        'path',
        {
          d: 'm12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83Z'
        },
        []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'm22 17.65-9.17 4.16a2 2 0 0 1-1.66 0L2 17.65',
          transition: DEFAULT_TRANSITION,
          variants: {
            normal: { y: 0 },
            firstState: { y: -9 },
            secondState: { y: 0 }
          },
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'm22 12.65-9.17 4.16a2 2 0 0 1-1.66 0L2 12.65',
          transition: DEFAULT_TRANSITION,
          variants: {
            normal: { y: 0 },
            firstState: { y: -5 },
            secondState: { y: 0 }
          },
          initial: 'normal'
        },
        () => []
      )
    ])
}

export default icon
