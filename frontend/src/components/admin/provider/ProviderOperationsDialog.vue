<template>
  <BaseDialog
    :show="show"
    :title="t(historyMode ? 'admin.providers.operations.history' : 'admin.providers.operations.relogin')"
    width="wide"
    @close="close"
  >
    <div class="space-y-4">
      <p v-if="loading" class="text-sm text-gray-600 dark:text-dark-300" role="status">{{ t('common.loading') }}</p>
      <SettingsNotice v-if="error" tone="error">{{ error }}</SettingsNotice>
      <SettingsNotice v-if="refreshFailed" tone="warning">{{ t('admin.providers.operations.refreshFailed') }}</SettingsNotice>

      <template v-if="!loading && !historyMode && !submitted && previewLoaded">
        <p class="text-sm text-gray-700 dark:text-dark-200">
          {{ t('admin.providers.operations.previewSummary', { selected: previewItems.length, eligible: eligibleIds.length }) }}
        </p>
        <p v-if="eligibleIds.length > 0" class="text-sm text-gray-700 dark:text-dark-200">
          {{ t('admin.providers.operations.reloginConfirmHint') }}
        </p>
      </template>

      <div v-if="operations.length" class="overflow-hidden rounded-control border border-gray-200 dark:border-dark-600">
        <ProviderOperationList :operations="visibleOperations" flat />
        <Pagination
          v-if="operations.length > pageSize"
          v-model:page="operationsPage"
          :total="operations.length"
          :page-size="pageSize"
          :show-page-size-selector="false"
        />
      </div>
      <p v-if="!loading && !error && historyMode && !operations.length" class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('admin.providers.operations.historyEmpty') }}
      </p>

      <div v-if="displayedPreview.length" class="overflow-hidden rounded-control border border-gray-200 dark:border-dark-600">
        <ul class="divide-y divide-gray-100 dark:divide-dark-600" :aria-label="t('admin.providers.operations.previewLabel')">
          <li v-for="item in visiblePreview" :key="item.provider_id" class="px-3 py-2 text-sm">
            <div class="flex flex-wrap items-baseline justify-between gap-2">
              <span class="break-all font-medium text-gray-900 dark:text-gray-100">{{ item.name || `#${item.provider_id}` }}</span>
              <span :class="item.eligible ? 'text-emerald-700 dark:text-emerald-300' : 'text-amber-800 dark:text-amber-300'" class="text-xs font-medium">
                {{ t(item.eligible ? 'admin.providers.operations.eligible' : 'admin.providers.operations.skipped') }}
              </span>
            </div>
            <p v-if="item.reason" class="mt-1 break-words text-xs text-gray-700 dark:text-dark-200">{{ item.reason }}</p>
          </li>
        </ul>
        <Pagination
          v-if="displayedPreview.length > pageSize"
          v-model:page="previewPage"
          :total="displayedPreview.length"
          :page-size="pageSize"
          :show-page-size-selector="false"
        />
      </div>
    </div>

    <template #footer>
      <div class="flex flex-wrap justify-end gap-2">
        <button type="button" class="btn btn-secondary" :disabled="submitting" @click="close">
          {{ t(historyMode || submitted ? 'common.close' : 'common.cancel') }}
        </button>
        <button v-if="error && !previewLoaded" type="button" class="btn btn-primary" @click="load">
          {{ t('common.retry') }}
        </button>
        <button
          v-else-if="!historyMode && !submitted"
          type="button"
          class="btn btn-primary"
          data-testid="confirm-provider-relogin"
          :disabled="loading || submitting || !eligibleIds.length"
          @click="confirmRelogin"
        >
          {{ t(submitting ? 'common.processing' : 'admin.providers.operations.confirmRelogin') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { ProviderOperation, ProviderReloginPreviewItem } from '@/api/admin/providers'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import ProviderOperationList from './ProviderOperationList.vue'
import { isProviderOperationPending, useProviderOperationPolling } from '@/composables/useProviderOperationPolling'
import { extractApiErrorMessage } from '@/utils/apiError'

const props = defineProps<{
  show: boolean
  providerIds: number[] | null
}>()
const emit = defineEmits<{ close: []; 'relogin-finished': [] }>()
const { t } = useI18n()

const historyMode = computed(() => props.providerIds === null)
const loading = ref(false)
const submitting = ref(false)
const submitted = ref(false)
const previewLoaded = ref(false)
const error = ref('')
const previewItems = ref<ProviderReloginPreviewItem[]>([])
const skipped = ref<ProviderReloginPreviewItem[]>([])
const operations = ref<ProviderOperation[]>([])
const pageSize = 50
const operationsPage = ref(1)
const previewPage = ref(1)
const eligibleIds = computed(() => previewItems.value.filter(item => item.eligible).map(item => item.provider_id))
const displayedPreview = computed(() => submitted.value ? skipped.value : previewItems.value)
// 两张列表分页显示，确认仍使用整份预览里的账号。
const visiblePreview = computed(() => displayedPreview.value.slice((previewPage.value - 1) * pageSize, previewPage.value * pageSize))
const visibleOperations = computed(() => operations.value.slice((operationsPage.value - 1) * pageSize, operationsPage.value * pageSize))
const refreshFailed = useProviderOperationPolling(operations, () => props.show)

let loadController: AbortController | undefined

// 关闭弹窗后停止读取预览和历史。提交请求返回前保持弹窗打开。
const close = () => {
  if (!submitting.value) emit('close')
}

const load = async () => {
  loadController?.abort()
  const controller = new AbortController()
  loadController = controller
  const ids = props.providerIds
  error.value = ''
  loading.value = true
  try {
    if (ids === null) {
      const response = await adminAPI.providers.listOperations(undefined, controller.signal)
      if (!controller.signal.aborted) operations.value = response.operations
    } else {
      const response = await adminAPI.providers.previewRelogin(ids, controller.signal)
      if (!controller.signal.aborted) {
        previewItems.value = response.items
        previewLoaded.value = true
      }
    }
  } catch (err) {
    if (!controller.signal.aborted) error.value = extractApiErrorMessage(err, t('admin.providers.operations.loadFailed'))
  } finally {
    if (!controller.signal.aborted) loading.value = false
  }
}

const confirmRelogin = async () => {
  if (submitting.value || eligibleIds.value.length === 0) return
  submitting.value = true
  error.value = ''
  try {
    const response = await adminAPI.providers.startRelogin(eligibleIds.value)
    operations.value = response.operations
    skipped.value = [...previewItems.value.filter(item => !item.eligible), ...(response.skipped ?? [])]
    previewPage.value = 1
    submitted.value = true
  } catch (err) {
    error.value = extractApiErrorMessage(err, t('admin.providers.operations.reloginFailed'))
  } finally {
    submitting.value = false
  }
}

// 每次打开都从服务端读取，预览固定使用打开时的选中账号。
watch(() => [props.show, props.providerIds] as const, ([show]) => {
  loadController?.abort()
  submitted.value = false
  previewLoaded.value = false
  error.value = ''
  previewItems.value = []
  skipped.value = []
  operations.value = []
  previewPage.value = 1
  operationsPage.value = 1
  if (show) void load()
})

watch(() => submitted.value && operations.value.length > 0 && !operations.value.some(isProviderOperationPending), (finished) => {
  if (finished) emit('relogin-finished')
})

onUnmounted(() => loadController?.abort())
</script>
