<template>
  <RuleListEditor
    :items="rows"
    :add-label="t('admin.providers.headerOverride.addRow')"
    add-placement="footer"
    :empty-text="t('admin.providers.headerOverride.empty')"
    @add="addRow"
    @remove="removeRow"
  >
    <template #row="{ item: row }">
      <div class="grid min-w-0 grid-cols-1 gap-2 sm:grid-cols-2">
        <input
          v-model="row.name"
          type="text"
          class="input min-w-0 font-mono"
          :placeholder="t('admin.providers.headerOverride.namePlaceholder')"
          :aria-label="t('admin.providers.headerOverride.namePlaceholder')"
        />
        <input
          v-model="row.value"
          type="text"
          class="input min-w-0"
          :placeholder="t('admin.providers.headerOverride.valuePlaceholder')"
          :aria-label="t('admin.providers.headerOverride.valuePlaceholder')"
        />
      </div>
    </template>
    <template #footer>
      <div class="flex flex-wrap gap-2">
        <HeaderOverrideJsonTools :rows="rows" @update:rows="emit('update:rows', $event)" />
      </div>
      <p class="text-xs text-gray-500 dark:text-gray-400">
        {{ t('admin.providers.headerOverride.emptyValueHint') }}
      </p>
    </template>
  </RuleListEditor>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import RuleListEditor from '@/components/common/RuleListEditor.vue'
import HeaderOverrideJsonTools from './HeaderOverrideJsonTools.vue'
import type { HeaderOverrideRow } from './credentialsBuilder'

const props = defineProps<{
  rows: HeaderOverrideRow[]
}>()

const emit = defineEmits<{
  (e: 'update:rows', rows: HeaderOverrideRow[]): void
}>()

const { t } = useI18n()

const addRow = () => {
  emit('update:rows', [...props.rows, { name: '', value: '' }])
}

const removeRow = (index: number) => {
  emit('update:rows', props.rows.filter((_, i) => i !== index))
}
</script>
