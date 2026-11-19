import { onBeforeUnmount, onMounted, shallowRef, watch, type Ref } from 'vue'

export type IconAnimationPhase = 'idle' | 'play' | 'reset'

const INTERACTIVE_SELECTOR = [
  'button',
  'a[href]',
  'summary',
  'label',
  '[data-icon-trigger]',
  '[role="button"]',
  '[role="tab"]',
  '[role="menuitem"]',
  '[role="menuitemcheckbox"]',
  '[role="menuitemradio"]',
  '[role="option"]',
  '[role="checkbox"]',
  '[role="radio"]',
  '[role="switch"]'
].join(', ')

/** 图标动画绑定到最近的控件，悬停控件文字也会触发。 */
export function useIconAnimation(
  svgRef: Ref<SVGSVGElement | null>,
  enabled: () => boolean,
  animationActive: () => boolean,
  name: () => string,
  hasAnimatedArtwork: () => boolean
) {
  const phase = shallowRef<IconAnimationPhase>('idle')
  let cleanup: (() => void) | undefined
  let update: (() => void) | undefined

  onMounted(() => {
    const svg = svgRef.value
    if (!svg) return

    const trigger = svg.closest(INTERACTIVE_SELECTOR) ?? svg
    const labelledControl = trigger instanceof HTMLLabelElement ? trigger.control : null
    let hovered = false
    let focused = false
    let media: MediaQueryList | undefined
    let observer: MutationObserver | undefined

    const check: () => void = () => {
      const requested = animationActive() || (enabled() && (hovered || focused))
      if (!requested && phase.value === 'idle') {
        stopTracking()
        return
      }

      // 控件进入反馈状态后才观察减少动态效果、禁用和 inert 的变化。
      if (!observer) {
        media = window.matchMedia('(prefers-reduced-motion: reduce)')
        media.addEventListener('change', check)
        observer = new MutationObserver(check)
        let ancestor: Element | null = svg
        while (ancestor) {
          observer.observe(ancestor, {
            attributeFilter: ancestor === svg
              ? ['disabled', 'aria-disabled', 'inert', 'class']
              : ['disabled', 'aria-disabled', 'inert']
          })
          ancestor = ancestor.parentElement
        }
        if (labelledControl) {
          observer.observe(labelledControl, {
            attributeFilter: ['disabled', 'aria-disabled', 'inert']
          })
        }
      }

      const suppressed =
        (!enabled() && !animationActive()) ||
        media?.matches ||
        trigger.matches(':disabled') ||
        labelledControl?.matches(':disabled, [aria-disabled="true"], [inert]') ||
        Boolean(svg.closest('fieldset:disabled, [aria-disabled="true"], [inert]')) ||
        svg.classList.contains('animate-spin')

      if (suppressed) phase.value = 'idle'
      else if (requested) phase.value = 'play'
      else if (phase.value === 'play') phase.value = hasAnimatedArtwork() ? 'reset' : 'idle'

      if (!requested && phase.value === 'idle') stopTracking()
    }

    const stopTracking = () => {
      observer?.disconnect()
      observer = undefined
      media?.removeEventListener('change', check)
      media = undefined
    }
    const enter = (event: Event) => {
      // 触屏点击后可能出现模拟悬停，动画由实际聚焦或业务状态触发。
      if ('pointerType' in event && event.pointerType === 'touch') return
      hovered = true
      check()
    }
    const leave = () => {
      hovered = false
      check()
    }
    const focus = (event: Event) => {
      // 鼠标可以聚焦 tabindex=-1 的 SVG。把焦点交给可操作的控件。
      if (event.target instanceof SVGElement && svg.contains(event.target)) {
        const control = labelledControl ?? trigger
        if (control instanceof HTMLElement) control.focus({ preventScroll: true })
        const activeElement = svg.ownerDocument.activeElement
        if (activeElement instanceof SVGElement && svg.contains(activeElement)) {
          activeElement.blur()
          focused = false
          check()
          return
        }
      }
      focused = true
      check()
    }
    const blur = (event: Event) => {
      const { relatedTarget } = event as FocusEvent
      if (relatedTarget instanceof Node && trigger.contains(relatedTarget)) return
      focused = false
      check()
    }

    trigger.addEventListener('pointerenter', enter)
    trigger.addEventListener('pointerleave', leave)
    trigger.addEventListener('focusin', focus)
    trigger.addEventListener('focusout', blur)

    const unwatch = watch([enabled, animationActive, name], ([, , currentName], [, , previousName]) => {
      if (currentName !== previousName && phase.value === 'reset') phase.value = 'idle'
      check()
    })
    update = check
    cleanup = () => {
      stopTracking()
      unwatch()
      trigger.removeEventListener('pointerenter', enter)
      trigger.removeEventListener('pointerleave', leave)
      trigger.removeEventListener('focusin', focus)
      trigger.removeEventListener('focusout', blur)
    }
    check()
  })

  const finishReset = () => {
    if (phase.value !== 'reset') return
    phase.value = 'idle'
    update?.()
  }

  onBeforeUnmount(() => cleanup?.())
  return { phase, finishReset }
}
