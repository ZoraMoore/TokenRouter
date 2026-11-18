<template>
  <div
    v-segmented
    class="segmented flex-wrap"
    :class="block ? 'flex w-full' : 'max-w-full'"
    role="radiogroup"
    :aria-label="ariaLabel"
  >
    <button
      v-for="option in options"
      :key="String(option.value)"
      type="button"
      role="radio"
      :aria-checked="modelValue === option.value"
      :disabled="disabled || option.disabled"
      :data-testid="option.testid"
      class="segmented-item inline-flex items-center justify-center gap-1.5 px-3 py-1.5 text-sm disabled:cursor-not-allowed disabled:opacity-50"
      :class="[{ 'segmented-item-active': modelValue === option.value }, block && 'flex-1']"
      @click="select(option.value)"
    >
      <Icon v-if="option.icon" :name="option.icon" size="sm" :animate-on-hover="false" />
      {{ option.label }}
    </button>
  </div>
</template>

<script setup lang="ts" generic="T extends string | number | boolean | null">
import Icon from '@/components/icons/Icon.vue'
import type { IconName } from '@/components/icons/registry'
import { vSegmented } from '@/directives/segmented'

export interface SettingsSegmentedOption<V> {
  value: V
  label: string
  icon?: IconName
  disabled?: boolean
  /** 透传到选项按钮，供测试定位。 */
  testid?: string
}

// 设置表单中两到五个互斥选项统一使用分段控件，选中背景由 v-segmented 共享。
const props = defineProps<{
  modelValue: T
  options: SettingsSegmentedOption<T>[]
  ariaLabel: string
  /** 撑满整行并让选项等分宽度。 */
  block?: boolean
  disabled?: boolean
  /** 再次点击已选项时取消选择，回到 null。 */
  deselectable?: boolean
}>()
const emit = defineEmits<{ 'update:modelValue': [value: T | null] }>()

function select(value: T) {
  if (props.deselectable && props.modelValue === value) {
    emit('update:modelValue', null)
    return
  }
  emit('update:modelValue', value)
}
</script>
