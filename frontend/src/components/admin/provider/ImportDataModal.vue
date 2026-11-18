<template>
  <BaseDialog
    :show="show"
    :title="t('admin.providers.dataImportTitle')"
    width="normal"
    close-on-click-outside
    @close="handleClose"
  >
    <form id="import-data-form" class="space-y-4" @submit.prevent="handleImport">
      <div class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('admin.providers.dataImportHint') }}
      </div>
      <div
        class="rounded-control border border-amber-200 bg-amber-50 p-3 text-xs text-amber-600 dark:border-amber-800 dark:bg-amber-900/20 dark:text-amber-400"
      >
        {{ t('admin.providers.dataImportWarning') }}
      </div>

      <div>
        <label class="input-label">{{ t('admin.providers.dataImportFile') }}</label>
        <div
          class="flex flex-col gap-3 rounded-control border border-dashed px-4 py-3 transition-colors sm:flex-row sm:items-center sm:justify-between"
          :class="dragActive
            ? 'border-primary-400 bg-primary-50/70 dark:border-primary-500 dark:bg-primary-900/20'
            : 'border-gray-300 bg-gray-50 dark:border-dark-600 dark:bg-dark-800'"
          @dragenter.prevent="handleDragEnter"
          @dragover.prevent
          @dragleave.prevent="handleDragLeave"
          @drop.prevent="handleDrop"
        >
          <div class="min-w-0">
            <div class="truncate text-sm text-gray-700 dark:text-dark-200" :title="fileListTitle">
              {{ selectedFilesLabel || t('admin.providers.dataImportSelectFile') }}
            </div>
            <div class="text-xs text-gray-500 dark:text-dark-400">
              JSON (.json)
              <span v-if="files.length > 1"> · {{ fileListTitle }}</span>
            </div>
          </div>
          <div class="flex shrink-0 flex-wrap gap-2">
            <button
              type="button"
              class="btn btn-secondary inline-flex items-center gap-1.5"
              @click="openFilePicker"
            >
              <Icon name="upload" size="sm" />
              {{ t('common.chooseFile') }}
            </button>
          </div>
        </div>
        <input
          ref="fileInput"
          type="file"
          class="hidden"
          accept="application/json,.json"
          multiple
          @change="handleFileChange"
        />
      </div>

      <div>
        <label class="input-label">{{ t('admin.providers.dataImportPasteData') }}</label>
        <textarea
          v-model="pastedDataText"
          class="input min-h-40 resize-y font-mono text-xs leading-5"
          :placeholder="pasteDataPlaceholder"
          spellcheck="false"
        />
        <div class="mt-1 text-xs text-gray-500 dark:text-dark-400">
          {{ t('admin.providers.dataImportPasteHint') }}
        </div>
      </div>

      <div
        v-if="result"
        class="space-y-2 rounded-surface border border-gray-200 p-4 dark:border-dark-700"
      >
        <div class="text-sm font-medium text-gray-900 dark:text-white">
          {{ t('admin.providers.dataImportResult') }}
        </div>
        <div class="text-sm text-gray-700 dark:text-dark-300">
          {{ t('admin.providers.dataImportResultSummary', result) }}
        </div>

        <div v-if="errorItems.length" class="mt-2">
          <div class="text-sm font-medium text-red-600 dark:text-red-400">
            {{ t('admin.providers.dataImportErrors') }}
          </div>
          <div
            class="mt-2 max-h-48 overflow-auto rounded-control bg-gray-50 p-3 font-mono text-xs dark:bg-dark-800"
          >
            <div v-for="(item, idx) in errorItems" :key="idx" class="whitespace-pre-wrap">
              {{ item.kind }} {{ item.name || item.proxy_key || '-' }} - {{ item.message }}
            </div>
          </div>
        </div>
      </div>
    </form>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button class="btn btn-secondary" type="button" :disabled="importing" @click="handleClose">
          {{ t('common.cancel') }}
        </button>
        <button
          class="btn btn-primary"
          type="submit"
          form="import-data-form"
          :disabled="importing"
        >
          {{ importing ? t('admin.providers.dataImporting') : t('admin.providers.dataImportButton') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Icon from '@/components/icons/Icon.vue'
import { adminAPI } from '@/api/admin'
import { useAppStore } from '@/stores/app'
import type { AdminDataImportResult, AdminDataPayload } from '@/types'

interface Props {
  show: boolean
}

interface Emits {
  (e: 'close'): void
  (e: 'imported'): void
}

const props = defineProps<Props>()
const emit = defineEmits<Emits>()

const { t } = useI18n()
const appStore = useAppStore()

const importing = ref(false)
const files = ref<File[]>([])
const dragDepth = ref(0)
const pastedDataText = ref('')
const dragActive = computed(() => dragDepth.value > 0)
const hasCreatedData = ref(false)
const result = ref<AdminDataImportResult | null>(null)
const pasteDataPlaceholder = '{\n  "type": "tokenrouter-data",\n  "version": 2,\n  "providers": [],\n  "proxies": []\n}'

const fileInput = ref<HTMLInputElement | null>(null)
const selectedFilesLabel = computed(() => {
  if (files.value.length === 0) return ''
  if (files.value.length === 1) return files.value[0]?.name || ''
  return t('admin.providers.selectedCount', { count: files.value.length })
})
const fileListTitle = computed(() => files.value.map((item) => item.name).join(', '))

const errorItems = computed(() => result.value?.errors || [])

watch(
  () => props.show,
  (open) => {
    if (open) {
      files.value = []
      dragDepth.value = 0
      pastedDataText.value = ''
      hasCreatedData.value = false
      result.value = null
      if (fileInput.value) {
        fileInput.value.value = ''
      }
    }
  }
)

const openFilePicker = () => {
  fileInput.value?.click()
}

const handleFileChange = (event: Event) => {
  const target = event.target as HTMLInputElement
  setSelectedFiles(target.files)
  target.value = ''
}

const handleClose = () => {
  if (importing.value) return
  if (hasCreatedData.value) {
    hasCreatedData.value = false
    emit('imported')
  }
  emit('close')
}

const isJsonFile = (sourceFile: File) => {
  const name = sourceFile.name.toLowerCase()
  return name.endsWith('.json') || sourceFile.type === 'application/json'
}

const setSelectedFiles = (sourceFiles: FileList | File[] | null | undefined) => {
  if (importing.value) return
  const incoming = Array.from(sourceFiles || [])
  const picked = incoming.filter(isJsonFile)
  if (!picked.length) {
    appStore.showError(t('admin.providers.dataImportSelectFile'))
    return
  }
  if (picked.length < incoming.length) {
    appStore.showWarning(
      t('admin.providers.dataImportIgnoredFiles', { count: incoming.length - picked.length })
    )
  }
  files.value = picked
  result.value = null
}

const handleDragEnter = () => {
  if (importing.value) return
  dragDepth.value += 1
}

const handleDragLeave = () => {
  dragDepth.value = Math.max(0, dragDepth.value - 1)
}

const handleDrop = (event: DragEvent) => {
  dragDepth.value = 0
  if (importing.value) return
  setSelectedFiles(event.dataTransfer?.files)
}

const readFileAsText = async (sourceFile: File): Promise<string> => {
  if (typeof sourceFile.text === 'function') {
    return sourceFile.text()
  }

  if (typeof sourceFile.arrayBuffer === 'function') {
    const buffer = await sourceFile.arrayBuffer()
    return new TextDecoder().decode(buffer)
  }

  return await new Promise<string>((resolve, reject) => {
    const reader = new FileReader()
    reader.onload = () => resolve(String(reader.result ?? ''))
    reader.onerror = () => reject(reader.error || new Error('Failed to read file'))
    reader.readAsText(sourceFile)
  })
}

const SUPPORTED_DATA_TYPES = ['tokenrouter-data', 'sub2api-data']
const SUPPORTED_DATA_VERSION = 2

// 旧账号文件先转换为提供商集合，多文件合并使用同一种格式。
const normalizeDataPayload = (payload: unknown): AdminDataPayload | null => {
  if (!payload || typeof payload !== 'object' || Array.isArray(payload)) return null
  const candidate = payload as Record<string, unknown>
  if ('accounts' in candidate) {
    const legacyFields = ['type', 'version', 'exported_at', 'proxies', 'accounts', 'skipped_shadows']
    if (
      'providers' in candidate ||
      !Array.isArray(candidate.accounts) ||
      (candidate.type != null && candidate.type !== '' && candidate.type !== 'sub2api-data') ||
      (candidate.version != null && candidate.version !== 1) ||
      (candidate.proxies != null && !Array.isArray(candidate.proxies)) ||
      (candidate.exported_at != null && typeof candidate.exported_at !== 'string') ||
      Object.keys(candidate).some((key) => !legacyFields.includes(key))
    ) {
      return null
    }
    const { accounts, ...metadata } = candidate
    return {
      ...metadata,
      type: 'tokenrouter-data',
      version: SUPPORTED_DATA_VERSION,
      exported_at: typeof candidate.exported_at === 'string' ? candidate.exported_at : '',
      proxies: Array.isArray(candidate.proxies) ? candidate.proxies : [],
      providers: accounts
    } as AdminDataPayload
  }
  if (
    !SUPPORTED_DATA_TYPES.includes(candidate.type as string)
  ) {
    return null
  }
  if (
    candidate.version !== SUPPORTED_DATA_VERSION
  ) {
    return null
  }
  return Array.isArray(candidate.proxies) && Array.isArray(candidate.providers)
    ? payload as AdminDataPayload
    : null
}

const mergeDataPayloads = (payloads: AdminDataPayload[]): AdminDataPayload => {
  const [firstPayload] = payloads
  if (payloads.length === 1 && firstPayload) return firstPayload

  return {
    type: payloads.find((item) => typeof item.type === 'string')?.type,
    version: payloads.find((item) => typeof item.version === 'number')?.version,
    exported_at: new Date().toISOString(),
    proxies: payloads.flatMap((item) => item.proxies),
    providers: payloads.flatMap((item) => item.providers),
    skipped_shadows: payloads.reduce((sum, item) => {
      const count = Number(item.skipped_shadows || 0)
      return Number.isFinite(count) ? sum + count : sum
    }, 0)
  }
}

const handleImport = async () => {
  importing.value = true
  try {
    const pastedText = pastedDataText.value.trim()
    if (!pastedText && files.value.length === 0) {
      appStore.showError(t('admin.providers.dataImportSelectSource'))
      return
    }

    let dataPayload: AdminDataPayload
    if (pastedText) {
      // 粘贴内容优先，便于用户覆盖已选择的文件内容。
      const parsed = normalizeDataPayload(JSON.parse(pastedText))
      if (!parsed) {
        appStore.showError(t('admin.providers.dataImportInvalidFile', { name: 'JSON' }))
        return
      }
      dataPayload = parsed
    } else {
      const dataPayloads: AdminDataPayload[] = []
      for (const sourceFile of files.value) {
        let parsed: unknown
        try {
          parsed = JSON.parse(await readFileAsText(sourceFile))
        } catch {
          appStore.showError(
            t('admin.providers.dataImportParseFailedFile', { name: sourceFile.name })
          )
          return
        }
        const normalized = normalizeDataPayload(parsed)
        if (!normalized) {
          appStore.showError(t('admin.providers.dataImportInvalidFile', { name: sourceFile.name }))
          return
        }
        dataPayloads.push(normalized)
      }
      dataPayload = mergeDataPayloads(dataPayloads)
    }

    const res = await adminAPI.providers.importData({
      data: dataPayload,
    })

    result.value = res

    const msgParams: Record<string, unknown> = {
      provider_created: res.provider_created,
      provider_failed: res.provider_failed,
      proxy_created: res.proxy_created,
      proxy_reused: res.proxy_reused,
      proxy_failed: res.proxy_failed,
    }
    if (res.provider_failed > 0 || res.proxy_failed > 0) {
      // 部分成功也创建了数据；弹窗关闭时通过 imported 通知父组件刷新列表。
      if (res.provider_created > 0 || res.proxy_created > 0) {
        hasCreatedData.value = true
      }
      appStore.showError(t('admin.providers.dataImportCompletedWithErrors', msgParams))
    } else {
      appStore.showSuccess(t('admin.providers.dataImportSuccess', msgParams))
      emit('imported')
    }
  } catch (error: any) {
    if (error instanceof SyntaxError) {
      appStore.showError(t('admin.providers.dataImportParseFailed'))
    } else {
      appStore.showError(error?.message || t('admin.providers.dataImportFailed'))
    }
  } finally {
    importing.value = false
  }
}
</script>
