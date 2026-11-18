<template>
  <RuleListEditor
    :items="modelValue"
    :title="title"
    :hint="hint"
    :title-style="titleStyle"
    :add-label="addLabel"
    :add-placement="addPlacement"
    :empty-text="emptyText"
    :max="max"
    :disabled="disabled"
    :error="error"
    :test-id="testId"
    @add="addRow"
    @remove="removeRow"
  >
    <template v-if="$slots['title-suffix']" #title-suffix>
      <slot name="title-suffix" />
    </template>
    <template v-if="$slots['header-actions']" #header-actions>
      <slot name="header-actions" />
    </template>
    <template v-if="$slots['header-extra']" #header-extra>
      <slot name="header-extra" />
    </template>
    <template #row="{ item, index }">
      <div
        class="grid min-w-0 grid-cols-1 items-start gap-2 sm:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]"
      >
        <div class="min-w-0">
          <input
            v-model="item.from"
            type="text"
            class="input min-w-0"
            :class="{ 'font-mono': monospace, 'input-error': fieldErrors?.[index]?.from }"
            :placeholder="sourcePlaceholder"
            :aria-label="sourceLabel"
            :aria-invalid="fieldErrors?.[index]?.from ? 'true' : undefined"
            :disabled="disabled"
            :data-testid="testId ? `${testId}-source-${index}` : undefined"
            @input="publish"
          />
          <p v-if="fieldErrors?.[index]?.from" class="input-error-text" role="alert">
            {{ fieldErrors[index]?.from }}
          </p>
        </div>
        <div class="hidden h-9 items-center text-gray-400 sm:flex dark:text-dark-400">
          <Icon name="arrowRight" size="sm" aria-hidden="true" />
        </div>
        <div class="min-w-0">
          <input
            v-model="item.to"
            type="text"
            class="input min-w-0"
            :class="{ 'font-mono': monospace, 'input-error': fieldErrors?.[index]?.to }"
            :placeholder="targetPlaceholder"
            :aria-label="targetLabel"
            :aria-invalid="fieldErrors?.[index]?.to ? 'true' : undefined"
            :disabled="disabled"
            :data-testid="testId ? `${testId}-target-${index}` : undefined"
            @input="publish"
          />
          <p v-if="fieldErrors?.[index]?.to" class="input-error-text" role="alert">
            {{ fieldErrors[index]?.to }}
          </p>
        </div>
      </div>
    </template>
    <template v-if="$slots.footer" #footer>
      <slot name="footer" />
    </template>
  </RuleListEditor>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import RuleListEditor from './RuleListEditor.vue'
import type { ModelMappingRow } from '@/utils/modelMappingRules'

/** ModelMappingFieldErrors 是已翻译的逐字段错误，与行按下标对齐。 */
export type ModelMappingFieldErrors = Array<{ from?: string; to?: string } | undefined>

const props = withDefaults(
  defineProps<{
    modelValue: ModelMappingRow[]
    sourceLabel: string
    targetLabel: string
    sourcePlaceholder?: string
    targetPlaceholder?: string
    title?: string
    hint?: string
    titleStyle?: 'label' | 'section'
    addLabel?: string
    addPlacement?: 'header' | 'footer'
    emptyText?: string
    max?: number
    disabled?: boolean
    monospace?: boolean
    fieldErrors?: ModelMappingFieldErrors
    error?: string
    testId?: string
  }>(),
  {
    titleStyle: 'label',
    addPlacement: 'header',
    monospace: true,
  },
)

const emit = defineEmits<{
  'update:modelValue': [rows: ModelMappingRow[]]
  add: [row: ModelMappingRow]
  remove: [row: ModelMappingRow, index: number]
}>()

// 输入时原地修改行对象以保持行身份，再发出新数组让父级感知变化。
const publish = () => {
  emit('update:modelValue', [...props.modelValue])
}

const addRow = () => {
  const row: ModelMappingRow = { from: '', to: '' }
  emit('update:modelValue', [...props.modelValue, row])
  emit('add', row)
}

const removeRow = (index: number) => {
  const row = props.modelValue[index]
  emit(
    'update:modelValue',
    props.modelValue.filter((_, rowIndex) => rowIndex !== index),
  )
  if (row) emit('remove', row, index)
}
</script>
