<template>
  <div class="flex min-w-0 flex-nowrap items-center gap-2">
    <SearchInput
      :model-value="searchQuery"
      :placeholder="t('admin.providers.searchProviders')"
      class="min-w-0 flex-1 sm:flex-none sm:w-56 lg:w-52"
      @update:model-value="$emit('update:searchQuery', $event)"
      @search="$emit('change')"
    />

    <FilterDropdown :active-count="activeFilterCount" :columns="2" :description="t('admin.providers.filterHint')" @reset="clearFilters">
      <FilterField :label="t('admin.providers.columns.platform')">
        <Select :model-value="filters.platform" :options="pOpts" @update:model-value="updatePlatform" @change="$emit('change')" />
      </FilterField>
      <FilterField :label="t('admin.providers.columns.type')">
        <Select :model-value="filters.type" :options="tOpts" @update:model-value="updateType" @change="$emit('change')" />
      </FilterField>
      <FilterField :label="t('admin.providers.columns.status')">
        <Select :model-value="filters.status" :options="sOpts" @update:model-value="updateStatus" @change="$emit('change')" />
      </FilterField>
      <FilterField :label="t('admin.providers.privacyFilter')">
        <Select :model-value="filters.privacy_mode" :options="privacyOpts" @update:model-value="updatePrivacyMode" @change="$emit('change')" />
      </FilterField>
      <FilterField :label="t('admin.providers.columns.groups')" full>
        <Select :model-value="filters.group" :options="gOpts" searchable @update:model-value="updateGroup" @change="$emit('change')" />
      </FilterField>
    </FilterDropdown>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Select from '@/components/common/Select.vue'
import FilterDropdown from '@/components/common/FilterDropdown.vue'
import FilterField from '@/components/common/FilterField.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import type { AdminGroup } from '@/types'
import { CONCRETE_PLATFORM_OPTIONS } from '@/constants/platforms'

const props = defineProps<{ searchQuery: string; filters: Record<string, any>; groups?: AdminGroup[] }>()
const emit = defineEmits(['update:searchQuery', 'update:filters', 'change'])
const { t } = useI18n()

const filterKeys = ['platform', 'type', 'status', 'privacy_mode', 'group'] as const

const activeFilterCount = computed(() => filterKeys.filter((key) => String(props.filters?.[key] ?? '').trim() !== '').length)

const updatePlatform = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, platform: value }) }
const updateType = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, type: value }) }
const updateStatus = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, status: value }) }
const updatePrivacyMode = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, privacy_mode: value }) }
const updateGroup = (value: string | number | boolean | null) => { emit('update:filters', { ...props.filters, group: value }) }

const clearFilters = () => {
  const nextFilters = { ...props.filters }
  for (const key of filterKeys) nextFilters[key] = ''
  emit('update:filters', nextFilters)
  emit('change')
}

const pOpts = computed(() => [{ value: '', label: t('admin.providers.allPlatforms') }, ...CONCRETE_PLATFORM_OPTIONS])
const tOpts = computed(() => [
  { value: '', label: t('admin.providers.allTypes') },
  { value: 'oauth', label: t('admin.providers.oauthType') },
  { value: 'setup-token', label: t('admin.providers.setupToken') },
  { value: 'apikey', label: t('admin.providers.apiKey') },
  { value: 'service_account', label: t('admin.providers.serviceAccount') },
  { value: 'bedrock', label: 'AWS Bedrock' },
  { value: 'cosy', label: t('admin.providers.types.qoderCosy') }
])
const sOpts = computed(() => [
  { value: '', label: t('admin.providers.allStatus') },
  { value: 'active', label: t('admin.providers.status.active') },
  { value: 'inactive', label: t('admin.providers.status.inactive') },
  { value: 'error', label: t('admin.providers.status.error') },
  { value: 'rate_limited', label: t('admin.providers.status.rateLimited') },
  { value: 'temp_unschedulable', label: t('admin.providers.status.tempUnschedulable') },
  { value: 'unschedulable', label: t('admin.providers.status.unschedulable') }
])
const privacyOpts = computed(() => [
  { value: '', label: t('admin.providers.allPrivacyModes') },
  { value: '__unset__', label: t('admin.providers.privacyUnset') },
  { value: 'training_off', label: 'Privacy' },
  { value: 'training_set_cf_blocked', label: 'CF' },
  { value: 'training_set_failed', label: 'Fail' }
])
const gOpts = computed(() => [
  { value: '', label: t('admin.providers.allGroups') },
  { value: 'ungrouped', label: t('admin.providers.ungroupedGroup') },
  ...(props.groups || []).map(g => ({
    value: String(g.id),
    // 禁用分组仍可筛选，名称后缀显示禁用状态。
    label: g.status === 'active' ? g.name : `${g.name} (${t('common.inactive')})`
  }))
])
</script>
