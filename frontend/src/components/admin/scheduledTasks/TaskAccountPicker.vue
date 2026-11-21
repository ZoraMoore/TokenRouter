<template>
  <div class="space-y-2">
    <label for="scheduled-provider-search" class="input-label">{{ t('scheduledTasks.providerSearch') }}</label>
    <div class="input-icon-wrap">
      <span class="input-icon"><Icon name="search" size="sm" /></span>
      <input
        id="scheduled-provider-search" v-model="search" type="search" maxlength="160" class="input input-has-icon"
        :placeholder="t('common.searchPlaceholder')" :disabled="disabled" @keydown.enter.prevent
      />
    </div>

    <p class="text-xs font-medium text-gray-600 dark:text-dark-300">
      {{ t('scheduledTasks.selectedProviders') }} {{ t('common.selectedCount', { count: modelValue.length }) }}
    </p>
    <ul v-if="modelValue.length" class="flex max-h-28 flex-wrap gap-2 overflow-auto">
      <li v-for="id in modelValue" :key="id">
        <button
          type="button" class="badge badge-primary max-w-full gap-1"
          :aria-label="t('scheduledTasks.removeProvider', { name: nameFor(id) })"
          :disabled="disabled" @click="setSelected(id, false)"
        >
          <span class="break-all">{{ nameFor(id) }}</span>
          <Icon name="x" size="xs" />
        </button>
      </li>
    </ul>

    <SettingsNotice v-if="failed" tone="error">
      {{ t('scheduledTasks.providersLoadFailed') }}
      <button type="button" class="ml-2 underline" @click="refresh">{{ t('common.retry') }}</button>
    </SettingsNotice>
    <ContentSkeleton v-else-if="loading && !pageData" :rows="3" />
    <p v-else-if="!pageData?.items.length" class="text-sm text-gray-500 dark:text-dark-400">
      {{ t('scheduledTasks.noProviders') }}
    </p>
    <div v-else class="overflow-hidden rounded-surface border border-gray-200 dark:border-dark-600">
      <ul class="max-h-48 divide-y divide-gray-100 overflow-auto dark:divide-dark-700" :aria-busy="loading">
        <li v-for="account in pageData.items" :key="account.id">
          <label class="flex cursor-pointer items-center gap-2 px-4 py-2 text-sm hover:bg-gray-50 dark:hover:bg-dark-800">
            <input
              type="checkbox" class="h-4 w-4 shrink-0" :checked="modelValue.includes(account.id)" :disabled="disabled"
              :data-testid="`scheduled-provider-${account.id}`"
              @change="setSelected(account.id, ($event.target as HTMLInputElement).checked)"
            />
            <span class="min-w-0 flex-1 break-all">{{ account.name }}</span>
            <span class="shrink-0 text-xs text-gray-500 dark:text-dark-400">#{{ account.id }}</span>
          </label>
        </li>
      </ul>
      <!-- 页码按钮位于计划表单内，分页点击需要拦截按钮的默认提交。 -->
      <Pagination
        v-if="pageData.total > PAGE_SIZE" v-model:page="page"
        :total="pageData.total" :page-size="PAGE_SIZE" :show-page-size-selector="false" @click.prevent
      />
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { list } from '@/api/admin/providers'
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import Pagination from '@/components/common/Pagination.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import Icon from '@/components/icons/Icon.vue'
import { useKeyedResource } from '@/composables/useKeyedResource'
import { SEARCH_DEBOUNCE_MS } from '@/constants/ui'

const props = defineProps<{ modelValue: number[]; disabled?: boolean; knownNames?: ReadonlyMap<number, string> }>()
const emit = defineEmits<{ 'update:modelValue': [number[]] }>()
const { t } = useI18n()
const PAGE_SIZE = 20
const search = ref('')
const keyword = ref('')
const page = ref(1)
const names = new Map<number, string>()
const { data: pageData, loading, failed, refresh } = useKeyedResource(
  () => [page.value, keyword.value] as const,
  ([pageNumber, query], signal) => list(pageNumber, PAGE_SIZE, {
    platform: 'openai', type: 'oauth', lite: 'true', search: query || undefined,
  }, { signal }),
)

watch(search, (value, _previous, onCleanup) => {
  const timer = window.setTimeout(() => {
    keyword.value = value.trim()
    page.value = 1
  }, SEARCH_DEBOUNCE_MS)
  onCleanup(() => window.clearTimeout(timer))
})

// 搜索或翻页只替换候选项，已选账号及其名称独立保留。
watch(pageData, data => {
  for (const account of data?.items ?? []) names.set(account.id, account.name)
})

function nameFor(id: number) {
  return names.get(id) ?? props.knownNames?.get(id) ?? `#${id}`
}

function setSelected(id: number, checked: boolean) {
  const selected = new Set(props.modelValue)
  if (checked) selected.add(id)
  else selected.delete(id)
  emit('update:modelValue', [...selected])
}
</script>
