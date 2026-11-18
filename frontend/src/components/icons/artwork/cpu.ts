// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/cpu.tsx
import { h } from 'vue'
import { motion, type Transition } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const TRANSITION: Transition = {
  duration: 0.5,
  ease: 'easeInOut',
  repeat: 0
}

const Y_VARIANTS: Variants = {
  normal: {
    scale: 1,
    rotate: 0,
    opacity: 1
  },
  animate: {
    scaleY: [1, 1.5, 1],
    opacity: [1, 0.8, 1]
  }
}

const X_VARIANTS: Variants = {
  normal: {
    scale: 1,
    rotate: 0,
    opacity: 1
  },
  animate: {
    scaleX: [1, 1.5, 1],
    opacity: [1, 0.8, 1]
  }
}

const icon: IconDefinition = {
  name: 'cpu',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        'rect',
        {
          height: '16',
          rx: '2',
          width: '16',
          x: '4',
          y: '4'
        },
        []
      ),
      h(
        'rect',
        {
          height: '6',
          rx: '1',
          width: '6',
          x: '9',
          y: '9'
        },
        []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M15 2v2',
          transition: TRANSITION,
          variants: Y_VARIANTS,
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M15 20v2',
          transition: TRANSITION,
          variants: Y_VARIANTS,
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M2 15h2',
          transition: TRANSITION,
          variants: X_VARIANTS,
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M2 9h2',
          transition: TRANSITION,
          variants: X_VARIANTS,
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M20 15h2',
          transition: TRANSITION,
          variants: X_VARIANTS,
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M20 9h2',
          transition: TRANSITION,
          variants: X_VARIANTS,
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M9 2v2',
          transition: TRANSITION,
          variants: Y_VARIANTS,
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'M9 20v2',
          transition: TRANSITION,
          variants: Y_VARIANTS,
          initial: 'normal'
        },
        () => []
      )
    ])
}

export default icon
