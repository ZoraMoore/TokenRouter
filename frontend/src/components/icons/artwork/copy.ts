// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/copy.tsx
import { h } from 'vue'
import { motion, type Transition } from 'motion-v'
import type { IconDefinition } from '../types'

const DEFAULT_TRANSITION: Transition = {
  type: 'spring',
  stiffness: 160,
  damping: 17,
  mass: 1
}

const icon: IconDefinition = {
  name: 'copy',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        motion.rect,
        {
          animate: controls,
          height: '14',
          rx: '2',
          ry: '2',
          transition: DEFAULT_TRANSITION,
          variants: {
            normal: { translateY: 0, translateX: 0 },
            animate: { translateY: -3, translateX: -3 }
          },
          width: '14',
          x: '8',
          y: '8',
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2',
          transition: DEFAULT_TRANSITION,
          variants: {
            normal: { x: 0, y: 0 },
            animate: { x: 3, y: 3 }
          },
          initial: 'normal'
        },
        () => []
      )
    ])
}

export default icon
