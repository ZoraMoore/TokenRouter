<template>
  <tr v-if="present">
    <td :colspan="colspan" class="border-0 p-0">
      <Collapse :open="open" appear unmount-on-hide @after-leave="finishLeave">
        <slot />
      </Collapse>
    </td>
  </tr>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import Collapse from './Collapse.vue'

// 表格使用 tr/td 结构，收起完成后隐藏空行及其分隔线。
const props = defineProps<{ open: boolean; colspan: number }>()
const present = ref(props.open)
watch(() => props.open, (open) => { if (open) present.value = true }, { flush: 'sync' })
function finishLeave() {
  if (!props.open) present.value = false
}
</script>
