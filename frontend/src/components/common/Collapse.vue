<template>
  <Transition
    name="collapse"
    :appear="appear"
    :css="animate && !skipAnimation && !reducedMotion"
    @before-enter="beforeEnter"
    @before-leave="beforeLeave"
    @after-enter="finishEnter"
    @after-leave="finishLeave"
    @enter-cancelled="moving = false"
    @leave-cancelled="moving = false"
  >
    <div
      v-show="open"
      class="motion-collapse"
      :class="{ 'motion-collapse-moving': moving }"
      :inert="!open || undefined"
      :aria-hidden="!open || undefined"
      @form-field-reveal.capture="revealImmediately"
      @onboarding-reveal.capture="revealImmediately"
    >
      <div class="motion-collapse-inner">
        <RetainedContent v-if="renderContent" :freeze="unmountOnHide && !open">
          <slot />
        </RetainedContent>
      </div>
    </div>
  </Transition>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { usePreferredReducedMotion } from '@vueuse/core'
import RetainedContent from './RetainedContent'
import { isolateLeavingElement, restoreEnteringElement } from '@/utils/leavingElement'

// @project-doc docs/architecture/frontend_ui_conventions.md#ui_motion
const props = withDefaults(defineProps<{
  open: boolean
  unmountOnHide?: boolean
  animate?: boolean
  appear?: boolean
}>(), { unmountOnHide: false, animate: true, appear: false })

const emit = defineEmits<{
  'after-enter': []
  'after-leave': []
}>()
const preference = usePreferredReducedMotion()
const reducedMotion = computed(() => preference.value === 'reduce')
const moving = ref(false)
const skipAnimation = ref(false)
const renderContent = ref(!props.unmountOnHide || props.open)

watch(() => [props.open, props.unmountOnHide], () => {
  // 先恢复内容再计算展开高度，退出完成前保留组件及其表单状态。
  if (props.open || !props.unmountOnHide) renderContent.value = true
}, { flush: 'sync' })

function beforeEnter(element: Element) {
  restoreEnteringElement(element)
  moving.value = true
}

function beforeLeave(element: Element) {
  // 原本按需卸载的必填字段退出校验；长期保留的折叠表单继续支持跨区定位。
  if (props.unmountOnHide) isolateLeavingElement(element)
  moving.value = true
}

function finishEnter() {
  moving.value = false
  if (props.open) emit('after-enter')
}

function finishLeave() {
  moving.value = false
  if (props.open) return
  if (props.unmountOnHide) renderContent.value = false
  emit('after-leave')
}

function revealImmediately() {
  // 校验和引导在 nextTick 后就要定位字段，因此此次展开不等待动画。
  skipAnimation.value = true
  void nextTick(() => { skipAnimation.value = false })
}
</script>
