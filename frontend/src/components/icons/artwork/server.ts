// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/server.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const TOP_RECT_VARIANTS: Variants = {
  normal: { y: 0 },
  animate: {
    y: [0, 12, 12, 0],
    transition: {
      duration: 0.9,
      ease: 'easeInOut',
      repeat: 0,
      times: [0, 0.35, 0.65, 1]
    }
  }
}

const BOTTOM_RECT_VARIANTS: Variants = {
  normal: { y: 0 },
  animate: {
    y: [0, -12, -12, 0],
    transition: {
      duration: 0.9,
      ease: 'easeInOut',
      repeat: 0,
      times: [0, 0.35, 0.65, 1]
    }
  }
}

const icon: IconDefinition = {
  name: 'server',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        motion.g,
        {
          animate: controls,
          initial: 'normal',
          variants: TOP_RECT_VARIANTS
        },
        () => [
          h(
            'rect',
            {
              height: '8',
              rx: '2',
              ry: '2',
              width: '20',
              x: '2',
              y: '2'
            },
            []
          ),
          h(
            'line',
            {
              x1: '6',
              x2: '10',
              y1: '6',
              y2: '6'
            },
            []
          )
        ]
      ),
      h(
        motion.g,
        {
          animate: controls,
          initial: 'normal',
          variants: BOTTOM_RECT_VARIANTS
        },
        () => [
          h(
            'rect',
            {
              height: '8',
              rx: '2',
              ry: '2',
              width: '20',
              x: '2',
              y: '14'
            },
            []
          ),
          h(
            'line',
            {
              x1: '6',
              x2: '10',
              y1: '18',
              y2: '18'
            },
            []
          )
        ]
      )
    ])
}

export default icon
