import { onBeforeUnmount, ref, toValue, watch, type MaybeRefOrGetter } from 'vue'
import { usePreferredReducedMotion } from '@vueuse/core'

// useCountUp 返回一个跟随 source 变化、从当前显示值滚动到新值的数字。
// 动画使用四次缓出，前段快、尾段慢；系统减少动态效果或环境不支持 rAF 时直接显示新值。
export function useCountUp(source: MaybeRefOrGetter<number>, duration: number) {
  const reducedMotion = usePreferredReducedMotion()
  const display = ref(toValue(source))
  let frame = 0

  const cancel = () => {
    if (frame) cancelAnimationFrame(frame)
    frame = 0
  }

  watch(() => toValue(source), (target) => {
    cancel()
    const from = display.value
    if (from === target || reducedMotion.value === 'reduce' || typeof requestAnimationFrame !== 'function') {
      display.value = target
      return
    }
    const startedAt = performance.now()
    const tick = (now: number) => {
      const progress = Math.min((now - startedAt) / duration, 1)
      const eased = 1 - Math.pow(1 - progress, 4)
      display.value = from + (target - from) * eased
      if (progress < 1) {
        frame = requestAnimationFrame(tick)
        return
      }
      display.value = target
      frame = 0
    }
    frame = requestAnimationFrame(tick)
  })

  onBeforeUnmount(cancel)

  return display
}
