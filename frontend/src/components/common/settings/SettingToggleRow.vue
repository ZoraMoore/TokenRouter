<template>
  <SettingRow
    :id="id"
    :label="label"
    :hint="hint"
    :help="help"
    :setting="setting"
  >
    <template v-if="$slots.hint" #hint>
      <slot name="hint" />
    </template>
    <Toggle
      :id="id"
      :model-value="modelValue"
      :disabled="disabled"
      :aria-label="label"
      :aria-describedby="hint ? `${id}-hint` : undefined"
      :data-setting="setting"
      :data-testid="testid"
      size="md"
      @update:model-value="emit('update:modelValue', $event)"
    />
  </SettingRow>
</template>

<script setup lang="ts">
import Toggle from '@/components/common/Toggle.vue'
import SettingRow from './SettingRow.vue'

// 所有布尔设置共用标签、说明和开关的位置，依赖字段由调用方紧随其后放置。
defineProps<{
  id: string
  label: string
  modelValue: boolean
  hint?: string
  help?: string
  setting?: string
  disabled?: boolean
  /** 透传到开关按钮，供测试定位。 */
  testid?: string
}>()
const emit = defineEmits<{ 'update:modelValue': [value: boolean] }>()
</script>
