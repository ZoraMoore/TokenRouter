<script lang="ts">
import { defineComponent, h, normalizeClass, ref, shallowRef, type PropType } from 'vue'
import AnimatedIconArtwork from './AnimatedIconArtwork'
import { icons, type IconName } from './registry'
import { artworkFocusAttrs, renderStaticIconNode } from './renderIconNode'
import { useIconAnimation } from './useIconAnimation'

type IconSize = 'xs' | 'sm' | 'md' | 'lg' | 'xl'
// 导航使用 18px，普通按钮使用 16px，展示图标按场景选择更大档位。
const SIZE_CLASSES: Record<IconSize, [string, string]> = {
  xs: ['h-3', 'w-3'],
  sm: ['h-4', 'w-4'],
  md: ['h-[18px]', 'w-[18px]'],
  lg: ['h-6', 'w-6'],
  xl: ['h-8', 'w-8']
}

export default defineComponent({
  name: 'Icon',
  inheritAttrs: false,
  props: {
    name: { type: String as PropType<IconName>, required: true },
    size: { type: String as PropType<IconSize>, default: 'md' },
    strokeWidth: { type: Number, default: 1.75 },
    animateOnHover: { type: Boolean, default: true },
    animationActive: { type: Boolean, default: false }
  },
  setup(props, { attrs, slots }) {
    const svgRef = ref<SVGSVGElement | null>(null)
    const animatedArtwork = shallowRef<InstanceType<typeof AnimatedIconArtwork> | null>(null)
    const { phase, finishReset } = useIconAnimation(
      svgRef,
      () => props.animateOnHover,
      () => props.animationActive,
      () => props.name,
      () => animatedArtwork.value !== null
    )

    const drawArtwork = (name: IconName) => {
      const definition = icons[name]
      if ('scale' in definition) {
        return h(
          'g',
          {
            key: name,
            ...artworkFocusAttrs,
            class: phase.value === 'play' ? 'icon-scale-feedback' : undefined
          },
          definition.render(renderStaticIconNode, props.strokeWidth)
        )
      }
      // 根 SVG 继续接收点击与焦点，播放时替换内部图形。
      if (phase.value === 'idle') {
        return definition.render(renderStaticIconNode, props.strokeWidth)
      }
      return h(AnimatedIconArtwork, {
        key: name,
        ref: animatedArtwork,
        definition,
        strokeWidth: props.strokeWidth,
        phase: () => (props.name === name ? phase.value : 'idle'),
        onReset: finishReset
      })
    }

    return () => {
      const className = normalizeClass(attrs.class)
      const [height, width] = SIZE_CLASSES[props.size]
      const labelled = Boolean(attrs['aria-label'] || attrs['aria-labelledby'])

      return h(
        'svg',
        {
          xmlns: 'http://www.w3.org/2000/svg',
          fill: 'none',
          viewBox: '0 0 24 24',
          stroke: 'currentColor',
          'stroke-width': props.strokeWidth,
          'stroke-linecap': 'round',
          'stroke-linejoin': 'round',
          'aria-hidden': labelled ? undefined : true,
          role: labelled ? 'img' : undefined,
          focusable: 'false',
          tabindex: -1,
          ...attrs,
          ref: svgRef,
          'data-animated-icon': icons[props.name].name,
          // 调用方传入尺寸时使用该尺寸，省略 size 档位的尺寸类。
          class: [
            'shrink-0 select-none',
            !attrs.height &&
              !/(?:^|\s)(?:\S+:)?(?:h-|size-)/.test(className) &&
              height,
            !attrs.width &&
              !/(?:^|\s)(?:\S+:)?(?:w-|size-)/.test(className) &&
              width,
            className
          ]
        },
        [drawArtwork(props.name), slots.default?.()]
      )
    }
  }
})
</script>

<style>
@keyframes icon-scale-feedback {
  50% {
    transform: scale(1.06);
  }
}

.icon-scale-feedback {
  transform-origin: 12px 12px;
  animation: icon-scale-feedback 400ms cubic-bezier(0.42, 0, 0.58, 1);
}

@media (prefers-reduced-motion: reduce) {
  .icon-scale-feedback {
    animation: none;
  }
}
</style>
