// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/arrow-right.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const PATH_VARIANTS: Variants = {
  normal: { d: 'M5 12h14' },
  animate: {
    d: ['M5 12h14', 'M5 12h9', 'M5 12h14'],
    transition: {
      duration: 0.4
    }
  }
}

const SECONDARY_PATH_VARIANTS: Variants = {
  normal: { d: 'm12 5 7 7-7 7', translateX: 0 },
  animate: {
    d: 'm12 5 7 7-7 7',
    translateX: [0, -3, 0],
    transition: {
      duration: 0.4
    }
  }
}

const icon: IconDefinition = {
  name: 'arrow-right',
  normal: 'normal',
  animate: ['animate'],
  render: (controls) =>
    h('g', {}, [
      h(
        motion.path,
        {
          animate: controls,
          d: 'M5 12h14',
          variants: PATH_VARIANTS,
          initial: 'normal'
        },
        () => []
      ),
      h(
        motion.path,
        {
          animate: controls,
          d: 'm12 5 7 7-7 7',
          variants: SECONDARY_PATH_VARIANTS,
          initial: 'normal'
        },
        () => []
      )
    ])
}

export default icon
