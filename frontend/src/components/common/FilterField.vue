<template>
  <div ref="rootRef" class="min-w-0" :class="full ? 'col-span-full' : ''">
    <label v-if="label" class="filter-field-label">{{ label }}</label>
    <slot />
  </div>
</template>

<script setup lang="ts">
import { computed, inject, onBeforeUnmount, provide, ref, shallowRef, type ComputedRef } from 'vue'
import { FILTER_FIELD_KEY, FILTER_PANEL_KEY, type FilterChip, type FilterControlState } from './filterPanel'

// FilterField 是 FilterDropdown 面板里的单个筛选条件，full 让字段占满整行。
// 内部的 Select 会自动上报当前选项；文本输入等其他控件通过 valueText 和 clear 事件接入已选条件标签。
const props = withDefaults(defineProps<{
  label?: string
  full?: boolean
  // valueText 非空表示条件已生效，内容显示在面板顶部的标签里。
  valueText?: string
  // emptyValue 指定 Select 未生效时的取值，默认取第一项“全部”。
  emptyValue?: string | number | boolean | null
}>(), {
  label: '',
  full: false,
  valueText: undefined,
  emptyValue: undefined,
})
const emit = defineEmits<{ (event: 'clear'): void }>()

const rootRef = ref<HTMLElement | null>(null)
const controlState = shallowRef<ComputedRef<FilterControlState> | null>(null)

provide(FILTER_FIELD_KEY, {
  emptyValue: () => props.emptyValue,
  bindControl: (state) => { controlState.value = state },
})

// 显式传入 valueText 时以它为准，否则使用内部 Select 上报的状态。
const chip = computed<FilterChip | null>(() => {
  if (props.valueText !== undefined) {
    if (!props.valueText) return null
    return { label: props.label, text: props.valueText, clear: () => emit('clear'), el: rootRef.value }
  }
  const state = controlState.value?.value
  if (!state?.active) return null
  return { label: props.label, text: state.text, clear: state.clear, el: rootRef.value }
})

const unregister = inject(FILTER_PANEL_KEY, null)?.register(chip)
onBeforeUnmount(() => unregister?.())
</script>

<style scoped>
/* 筛选面板字段标签比表单标签更轻，用中性灰小字，把视觉重心留给控件本身。 */
.filter-field-label {
  @apply mb-1.5 block text-xs font-medium text-gray-500 dark:text-dark-300;
}
</style>
