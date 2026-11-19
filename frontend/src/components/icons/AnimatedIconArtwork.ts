import { defineComponent, h, onBeforeUnmount, onMounted, watch, type PropType } from 'vue'
import { motion, useAnimationControls } from 'motion-v'
import { artworkFocusAttrs, renderStaticIconNode } from './renderIconNode'
import type { AnimatedIconDefinition, RenderIconNode } from './types'
import type { IconAnimationPhase } from './useIconAnimation'

/** 启动逐元素动画并在复位结束后通知外层换回静态图形。 */
export default defineComponent({
  name: 'AnimatedIconArtwork',
  props: {
    definition: { type: Object as PropType<AnimatedIconDefinition>, required: true },
    strokeWidth: { type: Number, required: true },
    phase: { type: Function as PropType<() => IconAnimationPhase>, required: true }
  },
  emits: ['reset'],
  setup(props, { emit }) {
    const controls = useAnimationControls()
    let generation = 0

    const draw: RenderIconNode = (tag, props, children) => {
      const { animate, ...attrs } = props
      if (attrs.initial === undefined) return renderStaticIconNode(tag, attrs, children)

      return h(motion[tag], {
        ...attrs,
        ...artworkFocusAttrs,
        animate: animate ? controls : undefined
      }, Array.isArray(children) ? () => children : children)
    }

    const update = async (phase: IconAnimationPhase) => {
      const current = ++generation
      controls.stop()
      if (phase === 'reset') {
        await controls.start(props.definition.normal)
        if (current === generation && props.phase() === 'reset') emit('reset')
      }
      if (phase === 'play') {
        for (const target of props.definition.animate) {
          if (current !== generation || props.phase() !== 'play') return
          await controls.start(target)
        }
      }
    }

    onMounted(() => {
      // 子节点订阅控制器后重放触发挂载的悬停、聚焦或业务状态。
      // 同一轮事件内离开并返回时，取消先前的序列并重新播放。
      watch(() => props.phase(), update, { immediate: true, flush: 'sync' })
    })
    onBeforeUnmount(() => {
      ++generation
      controls.stop()
    })

    return () => props.definition.render(draw, props.strokeWidth)
  }
})
