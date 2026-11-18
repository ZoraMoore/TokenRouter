<template>
  <section class="card min-w-0 self-start" data-testid="showcase-settings">
    <header class="card-header flex items-start gap-3">
      <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-surface bg-emerald-50 text-emerald-700 dark:bg-emerald-950/40 dark:text-emerald-300">
        <Icon name="eye" size="md" />
      </span>
      <div>
        <h2 class="text-base font-semibold text-gray-900 dark:text-dark-50">{{ t('pelicanTests.showcase.title') }}</h2>
        <p class="input-hint">{{ t('pelicanTests.showcase.hint') }}</p>
      </div>
    </header>
    <div class="space-y-4 p-4 sm:p-5">
      <SettingsNotice v-if="loadFailed" tone="error">
        {{ t('pelicanTests.showcase.loadFailed') }}
        <button type="button" class="ml-2 underline" data-testid="retry-showcase-settings" :disabled="loading" @click="refresh">{{ t('common.retry') }}</button>
      </SettingsNotice>
      <ContentSkeleton v-if="loading && !draft" variant="form" :rows="4" />
      <form v-if="draft" novalidate @submit.prevent="save">
        <fieldset class="space-y-4" :disabled="saving">
          <SettingToggleRow
            id="showcase-enabled" v-model="draft.enabled" testid="showcase-enabled" :disabled="saving"
            :label="t('pelicanTests.showcase.enabled')" :hint="t('pelicanTests.showcase.enabledHint')"
          />
          <SettingToggleRow
            id="showcase-api-enabled" v-model="draft.api_enabled" testid="showcase-api-enabled" :disabled="saving"
            :label="t('pelicanTests.showcase.apiEnabled')" :hint="t('pelicanTests.showcase.apiHint')"
          />
          <p v-if="!draft.enabled && draft.api_enabled" class="input-hint">{{ t('pelicanTests.showcase.requiresBoth') }}</p>
          <button type="button" class="inline-flex items-center gap-2 text-sm text-primary-700 hover:underline dark:text-primary-300" data-testid="showcase-api-info" @click="apiOpen = true">
            <Icon name="link" size="sm" />{{ t('pelicanTests.showcase.apiInfo') }}
          </button>
          <div>
            <label for="showcase-max-items" class="input-label">{{ t('pelicanTests.showcase.maxItems') }}</label>
            <div class="relative">
              <input id="showcase-max-items" v-model.number="draft.max_items" class="input pr-12" type="number" min="1" max="100" step="1" required aria-describedby="showcase-count-hint" />
              <span class="pointer-events-none absolute inset-y-0 right-3 flex items-center text-sm text-gray-500">{{ t('pelicanTests.showcase.countUnit') }}</span>
            </div>
            <p id="showcase-count-hint" class="input-hint">{{ t('pelicanTests.showcase.countHint') }}</p>
          </div>
          <SettingToggleRow
            id="showcase-auto-cleanup" v-model="draft.auto_cleanup" testid="showcase-auto-cleanup" :disabled="saving"
            :label="t('pelicanTests.showcase.cleanup')" :hint="t('pelicanTests.showcase.cleanupHint')"
          />
          <div v-if="draft.auto_cleanup">
            <label for="showcase-retention-days" class="input-label">{{ t('pelicanTests.showcase.retention') }}</label>
            <div class="relative">
              <input id="showcase-retention-days" v-model.number="draft.retention_days" class="input pr-12" type="number" min="1" max="90" step="1" required aria-describedby="showcase-days-hint" />
              <span class="pointer-events-none absolute inset-y-0 right-3 flex items-center text-sm text-gray-500">{{ t('pelicanTests.showcase.daysUnit') }}</span>
            </div>
            <p id="showcase-days-hint" class="input-hint">{{ t('pelicanTests.showcase.daysHint') }}</p>
          </div>
          <SettingsNotice v-if="error" tone="error">{{ t(error) }}</SettingsNotice>
          <div class="flex items-center justify-between gap-2 border-t border-gray-200 pt-4 dark:border-dark-700">
            <span class="text-xs text-gray-500 dark:text-dark-400" role="status">{{ t(dirty ? 'pelicanTests.showcase.unsaved' : 'pelicanTests.showcase.saved') }}</span>
            <button class="btn btn-primary" type="submit" data-testid="showcase-save" :disabled="saving || !dirty">
              <Icon v-if="saving" name="loader" size="sm" class="animate-spin" />{{ t(saving ? 'common.saving' : 'common.save') }}
            </button>
          </div>
        </fieldset>
      </form>
    </div>
    <PelicanShowcaseApiDialog v-if="apiOpen && saved" :enabled="saved.enabled && saved.api_enabled" @close="apiOpen = false" />
  </section>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getShowcaseSettings, updateShowcaseSettings, type PelicanShowcaseSettings } from '@/api/pelicanShowcase'
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import SettingToggleRow from '@/components/common/settings/SettingToggleRow.vue'
import Icon from '@/components/icons/Icon.vue'
import { setPelicanVisibility } from '@/composables/usePelicanVisibility'
import PelicanShowcaseApiDialog from './PelicanShowcaseApiDialog.vue'

const { t } = useI18n()
const emit = defineEmits<{ configured: [enabled: boolean] }>()
const saved = ref<PelicanShowcaseSettings | null>(null)
const draft = ref<PelicanShowcaseSettings | null>(null)
const loading = ref(false)
const loadFailed = ref(false)
const saving = ref(false)
const error = ref('')
const apiOpen = ref(false)
let read: AbortController | null = null
let active = true
const dirty = computed(() => !!draft.value && !!saved.value &&
  (Object.keys(saved.value) as Array<keyof PelicanShowcaseSettings>).some(key => saved.value?.[key] !== draft.value?.[key]))

async function refresh() {
  if (saving.value) return
  read?.abort()
  const controller = new AbortController()
  read = controller
  loading.value = true
  loadFailed.value = false
  try {
    const current = await getShowcaseSettings(controller.signal)
    if (controller.signal.aborted) return
    // 刷新时保留本地尚未保存的编辑，API 说明始终使用上一次服务端确认的设置。
    if (!dirty.value) draft.value = { ...current }
    saved.value = current
    emit('configured', current.enabled)
  } catch {
    if (!controller.signal.aborted) loadFailed.value = true
  } finally {
    if (!controller.signal.aborted) loading.value = false
  }
}

function inRange(value: number, upper: number): boolean {
  return Number.isInteger(value) && value >= 1 && value <= upper
}

async function save() {
  if (!draft.value || !saved.value || !dirty.value || saving.value) return
  const input = { ...draft.value }
  if (!inRange(input.max_items, 100)) {
    error.value = 'pelicanTests.showcase.invalidCount'
    return
  }
  if (!inRange(input.retention_days, 90)) {
    if (input.auto_cleanup) {
      error.value = 'pelicanTests.showcase.invalidDays'
      return
    }
    input.retention_days = saved.value.retention_days
  }
  read?.abort()
  loading.value = false
  saving.value = true
  error.value = ''
  try {
    const current = await updateShowcaseSettings(input)
    if (!active) return
    saved.value = current
    draft.value = { ...current }
    setPelicanVisibility(current)
    emit('configured', current.enabled)
  } catch {
    if (active) error.value = 'pelicanTests.showcase.saveFailed'
  } finally {
    if (active) saving.value = false
  }
}

defineExpose({ refresh })
onMounted(refresh)
onBeforeUnmount(() => {
  active = false
  read?.abort()
})
</script>
