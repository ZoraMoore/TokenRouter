// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/credit-card.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const CARD_VARIANTS: Variants = {
  normal: {
    x: 0,
    transition: {
      type: 'spring',
      stiffness: 280,
      damping: 18
    }
  },
  animate: {
    x: [0, -4, 1.5, 0],
    transition: {
      duration: 0.7,
      times: [0, 0.4, 0.75, 1],
      ease: 'easeInOut'
    }
  }
}

const icon: IconDefinition = {
  name: 'credit-card',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        motion.g,
        {
          animate: controls,
          initial: 'normal',
          variants: CARD_VARIANTS
        },
        () => [
          h(
            'rect',
            {
              height: '14',
              rx: '2',
              width: '20',
              x: '2',
              y: '5'
            },
            []
          ),
          h(
            'line',
            {
              x1: '2',
              x2: '22',
              y1: '10',
              y2: '10'
            },
            []
          )
        ]
      )
    ])
}

export default icon
