<template>
  <SettingsSection>
    <!-- 与分组的“对话中的图片生成”保持同一布局：标题、作用范围、全宽下拉和当前选项说明。 -->
    <div class="space-y-2">
      <template v-if="!hideTitle">
        <label :for="`${uid}-select`" class="input-label">{{ t('admin.protocols.imagePolicy') }}</label>
        <p :id="`${uid}-scope`" class="input-hint">{{ t('admin.providers.openai.codexImageToolDesc') }}</p>
      </template>
      <Select
        :id="`${uid}-select`"
        :aria-label="t('admin.protocols.imagePolicy')"
        :aria-describedby="hideTitle ? `${uid}-description` : `${uid}-scope ${uid}-description`"
        :data-testid="`${testIdPrefix}-select`"
        :model-value="modelValue"
        :options="options"
        @update:model-value="emit('update:modelValue', String($event) as CodexImageToolMode)"
      />
      <p :id="`${uid}-description`" class="input-hint">{{ descriptions[modelValue] }}</p>
    </div>
  </SettingsSection>
</template>

<script setup lang="ts">
import { computed, useId } from 'vue'
import { useI18n } from 'vue-i18n'

import Select from '@/components/common/Select.vue'
import SettingsSection from '@/components/common/settings/SettingsSection.vue'
import type { CodexImageToolMode } from '@/utils/codexImageToolMode'

withDefaults(defineProps<{
  modelValue: CodexImageToolMode
  testIdPrefix?: string
  /** 批量编辑由外层的应用开关行展示标题和说明。 */
  hideTitle?: boolean
}>(), {
  testIdPrefix: 'codex-image-tool',
})

const emit = defineEmits<{
  (event: 'update:modelValue', value: CodexImageToolMode): void
}>()

const { t } = useI18n()
const uid = useId()

// 后三种策略与分组含义一致，直接复用分组文案；“跟随”在提供商层的继承来源不同，单独说明。
const sharedModes = ['enabled', 'disabled', 'block'] as const
const options = computed(() => [
  { value: 'inherit', label: t('admin.providers.openai.codexImageToolInherit') },
  ...sharedModes.map(mode => ({
    value: mode,
    label: t(`admin.protocols.imagePolicyOptions.${mode}.label`),
  })),
])
const descriptions = computed<Record<CodexImageToolMode, string>>(() => ({
  inherit: t('admin.providers.openai.codexImageToolInheritDesc'),
  enabled: t('admin.protocols.imagePolicyOptions.enabled.description'),
  disabled: t('admin.protocols.imagePolicyOptions.disabled.description'),
  block: t('admin.protocols.imagePolicyOptions.block.description'),
}))
</script>
