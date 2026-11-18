// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/chart-column-increasing.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const LINE_VARIANTS: Variants = {
  visible: { pathLength: 1, opacity: 1 },
  hidden: { pathLength: 0, opacity: 0 }
}

const icon: IconDefinition = {
  name: 'chart-column-increasing',
  normal: 'visible',
  animate: [
    (i) => ({
      pathLength: 0,
      opacity: 0,
      transition: { delay: i * 0.1, duration: 0.3 }
    }),
    (i) => ({
      pathLength: 1,
      opacity: 1,
      transition: { delay: i * 0.1, duration: 0.3 }
    })
  ],
  render: (controls) =>
    h('g', {}, [
      h(
        motion.path,
        {
          animate: controls,
          custom: 1,
          d: 'M13 17V9',
          initial: 'visible',
          variants: LINE_VARIANTS
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          custom: 2,
          d: 'M18 17V5',
          initial: 'visible',
          variants: LINE_VARIANTS
        },
        () => []
      ),
      h(
        'path',
        {
          d: 'M3 3v16a2 2 0 0 0 2 2h16'
        },
        []
      ),
      h(
        motion.path,
        {
          animate: controls,
          custom: 0,
          d: 'M8 17v-3',
          initial: 'visible',
          variants: LINE_VARIANTS
        },
        () => []
      )
    ])
}

export default icon
