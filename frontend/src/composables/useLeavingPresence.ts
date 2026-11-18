import { ref, watch } from 'vue'

/** 按需创建弹窗，外壳退出动画完成后卸载组件。 */
export function useLeavingPresence(visible: () => boolean) {
  const present = ref(visible())
  watch(visible, (open) => {
    if (open) present.value = true
  }, { flush: 'sync' })
  function afterLeave() {
    if (!visible()) present.value = false
  }
  return { present, afterLeave }
}
