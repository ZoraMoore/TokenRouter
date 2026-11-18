// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/terminal.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const LINE_VARIANTS: Variants = {
  normal: { opacity: 1 },
  animate: {
    opacity: [1, 0, 1],
    transition: {
      duration: 0.8,
      repeat: 0,
      ease: 'linear'
    }
  }
}

const icon: IconDefinition = {
  name: 'terminal',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        'polyline',
        {
          points: '4 17 10 11 4 5'
        },
        []
      ),
      h(
        motion.line,
        {
          animate: controls,
          initial: 'normal',
          variants: LINE_VARIANTS,
          x1: '12',
          x2: '20',
          y1: '19',
          y2: '19'
        },
        () => []
      )
    ])
}

export default icon
