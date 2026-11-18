// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/clock.tsx
import { h } from 'vue'
import { motion, type Transition } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const HAND_TRANSITION: Transition = {
  duration: 0.6,
  ease: [0.4, 0, 0.2, 1]
}

const HAND_VARIANTS: Variants = {
  normal: {
    rotate: 0,
    originX: '0%',
    originY: '100%'
  },
  animate: {
    rotate: 360,
    originX: '0%',
    originY: '100%'
  }
}

const MINUTE_HAND_TRANSITION: Transition = {
  duration: 0.5,
  ease: 'easeInOut'
}

const MINUTE_HAND_VARIANTS: Variants = {
  normal: {
    rotate: 0,
    originX: '0%',
    originY: '100%'
  },
  animate: {
    rotate: 45,
    originX: '0%',
    originY: '100%'
  }
}

const icon: IconDefinition = {
  name: 'clock',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        'circle',
        {
          cx: '12',
          cy: '12',
          r: '10'
        },
        []
      ),
      h(
        motion.line,
        {
          animate: controls,
          initial: 'normal',
          transition: HAND_TRANSITION,
          variants: HAND_VARIANTS,
          x1: '12',
          x2: '12',
          y1: '12',
          y2: '6'
        },
        () => []
      ),
      h(
        motion.line,
        {
          animate: controls,
          initial: 'normal',
          transition: MINUTE_HAND_TRANSITION,
          variants: MINUTE_HAND_VARIANTS,
          x1: '12',
          x2: '16',
          y1: '12',
          y2: '12'
        },
        () => []
      )
    ])
}

export default icon
