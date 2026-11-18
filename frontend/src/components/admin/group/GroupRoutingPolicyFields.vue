<template>
  <div
    class="settings-section space-y-6"
    data-group-field="routing-policy"
  >
    <SettingsSection>
      <ModelMappingEditor
        :model-value="mappingRows"
        :title="t('admin.groups.routingPolicy.mapping')"
        :hint="t('admin.groups.routingPolicy.mappingHint')"
        title-style="section"
        :add-label="t('admin.groups.routingPolicy.addMapping')"
        :empty-text="t('admin.groups.routingPolicy.mappingEmpty')"
        :source-label="t('admin.groups.routingPolicy.source')"
        :target-label="t('admin.groups.routingPolicy.target')"
        :source-placeholder="t('admin.groups.routingPolicy.source')"
        :target-placeholder="t('admin.groups.routingPolicy.target')"
        :error="mappingError"
        @update:model-value="onMappingRowsChange"
      >
        <template #footer>
          <input
            class="sr-only"
            tabindex="-1"
            :value="mappingError ? '' : 'valid'"
            required
            :aria-label="t('admin.groups.routingPolicy.mapping')"
          />
        </template>
      </ModelMappingEditor>
    </SettingsSection>
    <SettingsSection>
      <SettingToggleRow
        :id="`${idPrefix}-restrict-models`"
        :model-value="value.restrict_models"
        :label="t('admin.groups.routingPolicy.restrict')"
        :hint="t('admin.groups.routingPolicy.allowlistHint')"
        setting="restrict_models"
        @update:model-value="update({ restrict_models: $event })"
      />
      <template v-if="value.restrict_models">
        <label :for="`${idPrefix}-restriction-source`" class="input-label">{{
          t('admin.groups.settings.restrictionSource')
        }}</label>
        <Select
          :id="`${idPrefix}-restriction-source`"
          :aria-label="t('admin.groups.settings.restrictionSource')"
          :model-value="value.restriction_model_source || 'group_mapped'"
          :options="sourceOptions"
          @update:model-value="
            update({
              restriction_model_source: String(
                $event,
              ) as GroupRoutingPolicy['restriction_model_source'],
            })
          "
        />
        <ModelTagInput
          :aria-label="t('admin.groups.settings.allowedModels')"
          :models="value.allowed_models"
          @update:models="update({ allowed_models: $event })"
        />
      </template>
    </SettingsSection>
  </div>
</template>
<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { GroupRoutingPolicy } from '@/types'
import Select from '@/components/common/Select.vue'
import SettingToggleRow from '@/components/common/settings/SettingToggleRow.vue'
import SettingsSection from '@/components/common/settings/SettingsSection.vue'
import ModelMappingEditor from '@/components/common/ModelMappingEditor.vue'
import ModelTagInput from '@/components/admin/pricing/ModelTagInput.vue'
import { findModelConflict } from '@/components/admin/pricing/types'
import { cloneRoutingPolicy } from './routingPolicy'
import {
  mappingRowsToRecord,
  recordToMappingRows,
  type ModelMappingRow,
} from '@/utils/modelMappingRules'

const props = withDefaults(
  defineProps<{ modelValue?: GroupRoutingPolicy; idPrefix?: string }>(),
  { idPrefix: 'group-model-policy' },
)
const emit = defineEmits<{ 'update:modelValue': [value: GroupRoutingPolicy] }>()
const { t } = useI18n()
const value = computed(() => cloneRoutingPolicy(props.modelValue))
const sourceOptions = computed(() =>
  ['requested', 'group_mapped', 'upstream'].map((key) => ({
    value: key,
    label: t(`admin.groups.routingPolicy.basis.${key}`),
  })),
)
// 本地行允许暂存空行和重复来源，发布后的映射对象不能表达这些中间状态。
const mappingRows = ref<ModelMappingRow[]>([])
let lastPublished = ''
watch(
  () => [props.modelValue?.model_mapping] as const,
  () => {
    const mapping = value.value.model_mapping
    const signature = JSON.stringify(mapping)
    if (signature === lastPublished) return
    mappingRows.value = recordToMappingRows(mapping)
  },
  { immediate: true, deep: true },
)
const mappingError = computed(() => {
  const sources = mappingRows.value.map((row) => row.from.trim())
  if (
    sources.some((source) => !source) ||
    mappingRows.value.some((row) => !row.to.trim())
  )
    return t('admin.groups.routingPolicy.incompleteMapping')
  return findModelConflict(sources)
    ? t('admin.groups.routingPolicy.conflict')
    : ''
})
function update(patch: Partial<GroupRoutingPolicy>) {
  emit('update:modelValue', { ...value.value, ...patch })
}
function onMappingRowsChange(rows: ModelMappingRow[]) {
  mappingRows.value = rows
  const mapping = mappingRowsToRecord(rows)
  lastPublished = JSON.stringify(mapping)
  update({ model_mapping: mapping })
}
</script>
