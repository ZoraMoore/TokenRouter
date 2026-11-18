// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/palette.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const DASH_LENGTH = 70

const DRAW_DURATION = 0.45

const DOT_STAGGER = 0.08

const DOTS = [
  { cx: 6.5, cy: 12.5 },
  { cx: 8.5, cy: 7.5 },
  { cx: 13.5, cy: 6.5 },
  { cx: 17.5, cy: 10.5 }
]

const OUTLINE_VARIANTS: Variants = {
  normal: {
    strokeDashoffset: 0
  },
  animate: {
    strokeDashoffset: [DASH_LENGTH, 0],
    transition: {
      duration: DRAW_DURATION,
      ease: [0.65, 0, 0.35, 1]
    }
  }
}

const DOTS_GROUP_VARIANTS: Variants = {
  normal: {},
  animate: {
    transition: {
      delayChildren: DRAW_DURATION,
      staggerChildren: DOT_STAGGER
    }
  }
}

const DOT_VARIANTS: Variants = {
  normal: {
    scale: 1,
    transition: { duration: 0.2 }
  },
  animate: {
    scale: [0, 1],
    transition: {
      damping: 10,
      stiffness: 300,
      type: 'spring'
    }
  }
}

const icon: IconDefinition = {
  name: 'palette',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        motion.path,
        {
          animate: controls,
          d: 'M12 2a1 1 0 0 0 0 20l.25 0a1.75 1.75 0 0 0 1.4-2.8l-.3-.4a1.75 1.75 0 0 1 1.4-2.8h2.25a5 5 0 0 0 5-5 10 9 0 0 0-10-9z',
          initial: 'normal',
          strokeDasharray: DASH_LENGTH,
          variants: OUTLINE_VARIANTS
        },
        () => []
      ),
      h(
        motion.g,
        {
          animate: controls,
          initial: 'normal',
          variants: DOTS_GROUP_VARIANTS
        },
        () => [
          DOTS.map((dot) =>
            h(
              motion.circle,
              {
                cx: dot.cx,
                cy: dot.cy,
                fill: 'currentColor',
                key: `${dot.cx}-${dot.cy}`,
                r: '.5',
                style: { transformBox: 'fill-box', transformOrigin: 'center' },
                variants: DOT_VARIANTS,
                initial: 'normal'
              },
              () => []
            )
          )
        ]
      )
    ])
}

export default icon
