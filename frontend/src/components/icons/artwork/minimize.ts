// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/minimize.tsx
import { h } from 'vue'
import { motion, type Transition } from 'motion-v'
import type { IconDefinition } from '../types'

const DEFAULT_TRANSITION: Transition = {
  type: 'spring',
  stiffness: 250,
  damping: 25
}

const icon: IconDefinition = {
  name: 'minimize',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        motion.path,
        {
          animate: controls,
          d: 'M8 3v3a2 2 0 0 1-2 2H3',
          transition: DEFAULT_TRANSITION,
          variants: {
            normal: { translateX: '0%', translateY: '0%' },
            animate: { translateX: '2px', translateY: '2px' }
          },
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M21 8h-3a2 2 0 0 1-2-2V3',
          transition: DEFAULT_TRANSITION,
          variants: {
            normal: { translateX: '0%', translateY: '0%' },
            animate: { translateX: '-2px', translateY: '2px' }
          },
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M3 16h3a2 2 0 0 1 2 2v3',
          transition: DEFAULT_TRANSITION,
          variants: {
            normal: { translateX: '0%', translateY: '0%' },
            animate: { translateX: '2px', translateY: '-2px' }
          },
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M16 21v-3a2 2 0 0 1 2-2h3',
          transition: DEFAULT_TRANSITION,
          variants: {
            normal: { translateX: '0%', translateY: '0%' },
            animate: { translateX: '-2px', translateY: '-2px' }
          },
          initial: 'normal'
        },
        () => []
      )
    ])
}

export default icon
