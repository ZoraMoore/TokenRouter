// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/chevrons-up-down.tsx
import { h } from 'vue'
import { motion, type Transition } from 'motion-v'
import type { IconDefinition } from '../types'

const DEFAULT_TRANSITION: Transition = {
  type: 'spring',
  stiffness: 250,
  damping: 25
}

const icon: IconDefinition = {
  name: 'chevrons-up-down',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        motion.path,
        {
          animate: controls,
          d: 'm7 15 5 5 5-5',
          initial: 'normal',
          transition: DEFAULT_TRANSITION,
          variants: {
            normal: { translateY: '0%' },
            animate: { translateY: '2px' }
          }
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'm7 9 5-5 5 5',
          initial: 'normal',
          transition: DEFAULT_TRANSITION,
          variants: {
            normal: { translateY: '0%' },
            animate: { translateY: '-2px' }
          }
        },
        () => []
      )
    ])
}

export default icon
