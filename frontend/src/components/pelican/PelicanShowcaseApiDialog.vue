<template>
  <BaseDialog :show="true" width="wide" :title="t('pelicanTests.showcase.apiTitle')" @close="emit('close')">
    <div class="min-w-0 space-y-4">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <span class="badge" :class="enabled ? 'badge-success' : 'badge-warning'" data-testid="showcase-api-status">
          {{ t(enabled ? 'pelicanTests.showcase.apiAvailable' : 'pelicanTests.showcase.apiUnavailable') }}
        </span>
        <RouterLink to="/keys" class="btn btn-secondary btn-sm" @click="emit('close')">
          <Icon name="key" size="sm" />{{ t('pelicanTests.showcase.manageKeys') }}
        </RouterLink>
      </div>
      <SettingsNotice v-if="!enabled">{{ t('pelicanTests.showcase.requiresBoth') }}</SettingsNotice>
      <p class="text-sm text-gray-700 dark:text-dark-200">{{ t('pelicanTests.showcase.readScope') }}</p>
      <dl class="space-y-3">
        <div v-for="entry in endpoints" :key="entry.url">
          <dt class="input-label">{{ entry.label }}</dt>
          <dd class="flex items-center gap-2 rounded-control bg-gray-50 p-2 dark:bg-dark-800">
            <code class="min-w-0 flex-1 break-all text-xs">GET / HEAD {{ entry.url }}</code>
            <button type="button" class="btn-icon-sm" :title="t('pelicanTests.showcase.copyUrl')" :aria-label="t('pelicanTests.showcase.copyUrl')" @click="copyToClipboard(entry.url)">
              <Icon name="copy" size="sm" />
            </button>
          </dd>
        </div>
      </dl>
      <p class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('pelicanTests.showcase.authHint') }}
        <code>Authorization: Bearer YOUR_API_KEY</code> {{ t('pelicanTests.showcase.or') }} <code>x-api-key: YOUR_API_KEY</code>
      </p>
      <div>
        <SettingsSegmented v-model="example" :ariaLabel="t('pelicanTests.showcase.examples')" :options="exampleOptions" />
        <div class="mt-2 overflow-hidden rounded-surface bg-dark-900 text-dark-100">
          <div class="flex items-center justify-between border-b border-dark-700 px-3 py-2 text-xs">
            <span class="font-mono">curl</span>
            <button type="button" class="btn-icon-sm" :aria-label="t('pelicanTests.showcase.copyCommand')" :title="t('pelicanTests.showcase.copyCommand')" @click="copyToClipboard(command)">
              <Icon name="copy" size="sm" />
            </button>
          </div>
          <pre class="overflow-x-auto p-3 text-xs leading-6"><code>{{ command }}</code></pre>
        </div>
      </div>
    </div>
    <template #footer>
      <button type="button" class="btn btn-secondary" data-testid="close-showcase-api" @click="emit('close')">{{ t('common.close') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { RouterLink } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { buildApiUrl } from '@/api/url'
import BaseDialog from '@/components/common/BaseDialog.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import SettingsSegmented from '@/components/common/settings/SettingsSegmented.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'

const props = defineProps<{ enabled: boolean; itemId?: number }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const { copyToClipboard } = useClipboard()
const example = ref<'list' | 'item' | 'headers' | null>('list')
const exampleOptions = computed(() => (['list', 'item', 'headers'] as const)
  .map(value => ({ value, label: t(`pelicanTests.showcase.example.${value}`) })))
// 示例使用部署配置中的 API 地址，调用者自行替换密钥与作品编号。
const manifestUrl = new URL(buildApiUrl('/public/pelican-showcase'), window.location.origin).href
const itemUrl = computed(() => manifestUrl + '/items/' + (props.itemId && Number.isSafeInteger(props.itemId) && props.itemId > 0 ? props.itemId : 'RESULT_ID'))
const endpoints = computed(() => [
  { label: t('pelicanTests.showcase.manifest'), url: manifestUrl },
  { label: t('pelicanTests.showcase.item'), url: itemUrl.value },
])
const command = computed(() => {
  const options = example.value === 'headers' ? '-I' : '--compressed -i'
  const header = example.value === 'item' ? 'x-api-key: YOUR_API_KEY' : 'Authorization: Bearer YOUR_API_KEY'
  const url = example.value === 'item' ? itemUrl.value : manifestUrl
  return [`curl ${options}`, `  -H "${header}"`, `  "${url}"`].join(' \\\n')
})
</script>
