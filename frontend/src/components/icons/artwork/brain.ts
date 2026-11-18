// 图形与逐元素动效移植自 Lucide Animated；许可与版本见 ../README.md。
// https://github.com/pqoqubbw/icons/blob/072c38b1b04ea738d90a084485ccaad4b890ddca/icons/brain.tsx
import { h } from 'vue'
import { motion } from 'motion-v'
import type { IconDefinition, IconVariants as Variants } from '../types'

const BRAIN_STEM_VARIANTS: Variants = {
  normal: { pathLength: 1, pathOffset: 0 },
  animate: {
    pathLength: [1, 0.4, 1],
    pathOffset: [0, 0.25, 0],
    transition: {
      duration: 1.4,
      repeat: 0,
      repeatType: 'mirror',
      ease: 'easeInOut'
    }
  }
}

const BRAIN_SIDE_VARIANTS: Variants = {
  normal: { pathLength: 1, pathOffset: 0 },
  animate: {
    pathLength: [1, 0.5, 1],
    pathOffset: [0, 0.25, 0],
    transition: {
      duration: 1.4,
      repeat: 0,
      repeatType: 'mirror',
      ease: 'easeInOut'
    }
  }
}

const BRAIN_TOP_ARC_VARIANTS: Variants = {
  normal: { pathLength: 1, pathOffset: 0 },
  animate: {
    pathLength: [1, 0.8, 1],
    pathOffset: [0, 0.07, 0],
    transition: {
      duration: 1.4,
      repeat: 0,
      repeatType: 'mirror',
      ease: 'easeInOut'
    }
  }
}

const BRAIN_LOWER_ARC_VARIANTS: Variants = {
  normal: { pathLength: 1, pathOffset: 0 },
  animate: {
    pathLength: [1, 0.8, 1],
    pathOffset: [0, 0.14, 0],
    transition: {
      duration: 1.4,
      repeat: 0,
      repeatType: 'mirror',
      ease: 'easeInOut'
    }
  }
}

const icon: IconDefinition = {
  name: 'brain',
  normal: 'normal',
  animate: ['animate'],
  render: (controls, strokeWidth) =>
    h(
      motion.g,
      {
        animate: controls,
        variants: {
          normal: {
            scale: 1,
            strokeWidth
          },
          animate: {
            scale: [1, 1.08, 1],
            strokeWidth: [strokeWidth, strokeWidth * 1.125, strokeWidth],
            transition: {
              duration: 1.4,
              repeat: 0,
              repeatType: 'mirror',
              ease: 'easeInOut'
            }
          }
        },
        initial: 'normal'
      },
      () => [
        h(
          motion.path,
          {
            animate: controls,
            d: 'M12 18V5',
            variants: BRAIN_STEM_VARIANTS,
            initial: 'normal'
          },
          () => []
        ),
        h(
          motion.path,
          {
            animate: controls,
            d: 'M15 13a4.17 4.17 0 0 1-3-4 4.17 4.17 0 0 1-3 4',
            variants: BRAIN_SIDE_VARIANTS,
            initial: 'normal'
          },
          () => []
        ),
        h(
          motion.path,
          {
            animate: controls,
            d: 'M12 5A3 3 0 1 1 17.598 6.5',
            variants: BRAIN_TOP_ARC_VARIANTS,
            initial: 'normal'
          },
          () => []
        ),
        h(
          motion.path,
          {
            animate: controls,
            d: 'M12 5A3 3 0 1 0 6.402 6.5',
            variants: BRAIN_TOP_ARC_VARIANTS,
            initial: 'normal'
          },
          () => []
        ),
        h(
          'path',
          {
            d: 'M17.997 5.125a4 4 0 0 1 2.526 5.77'
          },
          []
        ),
        h(
          motion.path,
          {
            animate: controls,
            d: 'M18 18a4 4 0 0 0 2-7.464',
            variants: BRAIN_LOWER_ARC_VARIANTS,
            initial: 'normal'
          },
          () => []
        ),
        h(
          'path',
          {
            d: 'M19.967 17.483A4 4 0 1 1 12 18a4 4 0 1 1-7.967-.517'
          },
          []
        ),
        h(
          motion.path,
          {
            animate: controls,
            d: 'M6 18a4 4 0 0 1-2-7.464',
            variants: BRAIN_LOWER_ARC_VARIANTS,
            initial: 'normal'
          },
          () => []
        ),
        h(
          'path',
          {
            d: 'M6.003 5.125a4 4 0 0 0-2.526 5.77'
          },
          []
        )
      ]
    )
}

export default icon
