// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/grip-vertical.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const CIRCLES = [
  { cx: 9, cy: 5 },
  { cx: 9, cy: 12 },
  { cx: 9, cy: 19 },
  { cx: 15, cy: 5 },
  { cx: 15, cy: 12 },
  { cx: 15, cy: 19 }
]

const ROWS = 3

const VARIANTS: Variants = {
  normal: {
    opacity: 1,
    scale: 1,
    transition: { duration: 0.25, ease: 'easeOut' }
  },
  animate: (data: { index: number }) => {
    const row = data.index % ROWS
    const col = Math.floor(data.index / ROWS)
    const delay = row * 0.15 + col * (ROWS * 0.15 - 0.2)
    return {
      opacity: [1, 0.4, 1],
      scale: [1, 0.85, 1],
      transition: { delay, duration: 1, ease: 'easeInOut' }
    }
  }
}

const icon: IconDefinition = {
  name: 'grip-vertical',
  normal: 'normal',
  animate: ['animate', 'normal'],
  render: (controls) =>
    h('g', {}, [
      CIRCLES.map((circle, index) =>
        h(
          motion.circle,
          {
            animate: controls,
            custom: { index },
            cx: circle.cx,
            cy: circle.cy,
            initial: 'normal',
            key: `${circle.cx}-${circle.cy}`,
            r: '1',
            variants: VARIANTS
          },
          () => []
        )
      )
    ])
}

export default icon
