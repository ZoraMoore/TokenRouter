<template>
  <SettingToggleRow
    :id="id"
    :model-value="modelValue === 'force_on'"
    :label="label"
    :hint="hint"
    :disabled="disabled"
    :testid="testId"
    @update:model-value="emit('update:modelValue', $event ? 'force_on' : 'force_off')"
  />
</template>
<script setup lang="ts">
import { useId } from 'vue'
import SettingToggleRow from '@/components/common/settings/SettingToggleRow.vue'
import type { OpenAICompactMode } from '@/types'

// 控件只展示管理员的明确开关，历史输入在进入控件前完成转换。
defineProps<{ modelValue: OpenAICompactMode; label: string; hint?: string; disabled?: boolean; testId?: string }>()
const emit = defineEmits<{ 'update:modelValue': [value: OpenAICompactMode] }>()
const id = useId()
</script>
