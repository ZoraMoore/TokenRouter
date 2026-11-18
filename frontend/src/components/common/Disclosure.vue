<template>
  <div>
    <button
      :id="`${id}-trigger`"
      type="button"
      :aria-expanded="open"
      :aria-controls="`${id}-content`"
      :class="summaryClass"
      @click="open = !open"
    >
      <Icon name="chevronRight" size="xs" class="inline-block shrink-0 transition-transform duration-normal" :class="{ 'rotate-90': open }" :animate-on-hover="false" />
      <slot name="summary" />
    </button>
    <Collapse :id="`${id}-content`" :open="open" :aria-labelledby="`${id}-trigger`">
      <slot />
    </Collapse>
  </div>
</template>

<script setup lang="ts">
import { ref, useId } from 'vue'
import Collapse from './Collapse.vue'
import Icon from '@/components/icons/Icon.vue'

// 折叠头使用原生按钮，保留 Enter、空格和展开状态的可访问性语义。
defineProps<{ summaryClass?: string }>()
const id = useId()
const open = ref(false)
</script>
