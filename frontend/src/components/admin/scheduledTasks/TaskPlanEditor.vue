<template>
  <BaseDialog
    :show="show" :title="t(plan ? 'scheduledTasks.editPlan' : 'scheduledTasks.createPlan')"
    :subtitle="timezone ? t('scheduledTasks.timezone', { timezone }) : undefined" @close="close"
  >
    <form id="scheduled-plan-form" novalidate class="space-y-4" @submit.prevent="save">
      <SettingsNotice v-if="formError" tone="error">{{ formError }}</SettingsNotice>
      <div>
        <label for="scheduled-plan-name" class="input-label">{{ t('scheduledTasks.fields.name') }}</label>
        <input id="scheduled-plan-name" v-model="form.name" type="text" class="input" maxlength="160" required :disabled="saving" />
      </div>
      <div class="grid gap-4 sm:grid-cols-2">
        <div>
          <label for="scheduled-plan-kind" class="input-label">{{ t('scheduledTasks.fields.kind') }}</label>
          <Select
            id="scheduled-plan-kind" :model-value="form.kind" :options="kindOptions"
            :aria-label="t('scheduledTasks.fields.kind')" :disabled="saving" @update:model-value="selectKind"
          />
        </div>
        <div>
          <label for="scheduled-plan-scope" class="input-label">{{ t('scheduledTasks.fields.scope') }}</label>
          <Select
            id="scheduled-plan-scope" :model-value="form.scope" :options="scopeOptions"
            :aria-label="t('scheduledTasks.fields.scope')" :disabled="saving" @update:model-value="selectScope"
          />
        </div>
      </div>
      <p class="text-xs text-gray-600 dark:text-dark-300">{{ t(`scheduledTasks.kindHelp.${form.kind}`) }}</p>

      <div v-if="form.scope === 'groups'" class="space-y-2">
        <ContentSkeleton v-if="groupsLoading && !groups" :rows="2" />
        <SettingsNotice v-if="groupsFailed" tone="error">
          {{ t('scheduledTasks.groupsLoadFailed') }}
          <button type="button" class="ml-2 underline" @click="refreshGroups">{{ t('common.retry') }}</button>
        </SettingsNotice>
        <fieldset v-if="groups" :disabled="saving" @keydown.enter.prevent>
          <GroupSelector v-model="form.group_ids" :groups="groups" />
        </fieldset>
        <p v-if="form.kind === 'iq' && form.group_ids.length > 1" class="text-xs text-gray-500 dark:text-dark-400">
          {{ t('scheduledTasks.groupOrder') }}
        </p>
      </div>
      <TaskAccountPicker v-if="form.scope === 'providers'" v-model="form.provider_ids" :disabled="saving" :known-names="previewNames" />
      <div v-if="form.kind === 'iq'">
        <label for="scheduled-plan-model" class="input-label">{{ t('scheduledTasks.fields.model') }}</label>
        <input
          id="scheduled-plan-model" v-model="form.model" class="input font-mono" list="scheduled-model-options"
          :placeholder="t('scheduledTasks.modelPlaceholder')" :disabled="saving" spellcheck="false" maxlength="128" required
        />
        <datalist id="scheduled-model-options">
          <option v-for="model in models" :key="model" :value="model" />
        </datalist>
      </div>
      <div>
        <label for="scheduled-plan-frequency" class="input-label">{{ t('scheduledTasks.fields.frequency') }}</label>
        <Select
          id="scheduled-plan-frequency" :model-value="frequency" :options="frequencyOptions"
          :aria-label="t('scheduledTasks.fields.frequency')" :disabled="saving" @update:model-value="selectFrequency"
        />
      </div>
      <div v-if="frequency === 'custom'">
        <label for="scheduled-plan-cron" class="input-label">{{ t('scheduledTasks.customCron') }}</label>
        <input
          id="scheduled-plan-cron" v-model="form.cron_expression" class="input font-mono" placeholder="0 */6 * * *"
          :disabled="saving" spellcheck="false" maxlength="120" required
        />
      </div>
      <div class="flex items-center justify-between gap-2">
        <div class="flex items-center gap-2">
          <Toggle v-model="form.enabled" :disabled="saving" size="sm" :aria-label="t('scheduledTasks.fields.status')" />
          <span class="text-sm">{{ t(form.enabled ? 'scheduledTasks.enabled' : 'scheduledTasks.paused') }}</span>
        </div>
        <button
          type="button" class="btn btn-secondary btn-sm" data-testid="scheduled-preview"
          :disabled="saving || previewLoading" @click="preview"
        >
          <Icon :name="previewLoading ? 'loader' : 'eye'" size="sm" :class="{ 'animate-spin': previewLoading }" />
          {{ t('scheduledTasks.preview') }}
        </button>
      </div>
      <div v-if="previewData" class="space-y-2 rounded-surface border border-gray-200 p-4 dark:border-dark-600" aria-live="polite">
        <p class="text-sm font-medium">
          {{ t('scheduledTasks.previewSummary', { total: previewData.total, eligible: previewData.eligible }) }}
        </p>
        <p v-if="form.kind === 'iq'" class="text-xs text-gray-600 dark:text-dark-300">
          {{ t('scheduledTasks.previewRequests', { min: previewData.eligible * 2, max: previewData.eligible * 3 }) }}
        </p>
        <p v-else class="text-xs text-gray-600 dark:text-dark-300">
          {{ t('scheduledTasks.previewAuthRequests', { count: previewData.eligible }) }}
        </p>
        <p v-if="!previewData.total" class="text-xs text-gray-500 dark:text-dark-400">{{ t('scheduledTasks.previewEmpty') }}</p>
        <ul v-else class="max-h-48 divide-y divide-gray-100 overflow-auto dark:divide-dark-700">
          <li v-for="item in previewItems" :key="item.provider_id" class="space-y-1 py-2 text-sm">
            <p class="break-all font-medium">{{ item.provider_name || `#${item.provider_id}` }}</p>
            <p class="break-words text-xs text-gray-500 dark:text-dark-400">
              <template v-if="item.group_id">{{ t('scheduledTasks.groupId', { id: item.group_id }) }} · </template>
              <code v-if="item.actual_model || item.model" class="font-mono">{{ item.actual_model || item.model }} · </code>
              {{ item.reason ? reasonLabel(item.reason) : t('scheduledTasks.eligible') }}
            </p>
          </li>
        </ul>
        <Pagination
          v-if="previewData.items.length > PREVIEW_PAGE_SIZE" v-model:page="previewPage" :total="previewData.items.length"
          :page-size="PREVIEW_PAGE_SIZE" :show-page-size-selector="false" @click.prevent
        />
      </div>
    </form>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="close">{{ t('common.cancel') }}</button>
        <button form="scheduled-plan-form" type="submit" class="btn btn-primary" :disabled="saving">
          {{ t(saving ? 'common.processing' : 'common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, shallowRef, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getOptions, getModelsListCandidates } from '@/api/admin/groups'
import { getAvailableModels } from '@/api/admin/providers'
import { previewPlan, savePlan, type ScheduledTaskDraft, type ScheduledTaskPlan, type ScheduledTaskPreview } from '@/api/admin/scheduledTasks'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import GroupSelector from '@/components/common/GroupSelector.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import { frequencyForCron, pelicanFrequencies, type PelicanFrequency } from '@/components/pelican/pelicanFrequency'
import { useKeyedResource } from '@/composables/useKeyedResource'
import { extractApiErrorMessage } from '@/utils/apiError'
import TaskAccountPicker from './TaskAccountPicker.vue'
import { taskDraft } from './taskPresentation'

const props = defineProps<{ show: boolean; plan: ScheduledTaskPlan | null; timezone: string }>()
const emit = defineEmits<{ close: []; saved: [ScheduledTaskPlan] }>()
const { t, te } = useI18n()
const PREVIEW_PAGE_SIZE = 16
const form = ref<ScheduledTaskDraft>(defaultDraft())
const frequency = ref<PelicanFrequency>('every15m')
const saving = ref(false)
const formError = ref('')
const previewData = shallowRef<ScheduledTaskPreview | null>(null)
const previewPage = ref(1)
const previewRequest = ref(0)
const previewLoading = ref(false)
const previewNames = ref(new Map<number, string>())
const previewItems = computed(() => {
  const offset = (previewPage.value - 1) * PREVIEW_PAGE_SIZE
  return previewData.value?.items.slice(offset, offset + PREVIEW_PAGE_SIZE) ?? []
})
const previewKey = computed(() => {
  if (!props.show) return ''
  const { kind, scope, group_ids, provider_ids, model } = taskDraft(form.value)
  return JSON.stringify({ kind, scope, group_ids, provider_ids, model })
})

const kindOptions = computed(() => ['relogin', 'iq'].map(value => ({ value, label: t(`scheduledTasks.kind.${value}`) })))
const scopeOptions = computed(() => (form.value.kind === 'relogin' ? ['all', 'groups', 'providers'] : ['groups', 'providers'])
  .map(value => ({ value, label: t(`scheduledTasks.scope.${value}`) })))
const frequencyOptions = computed(() => [...Object.keys(pelicanFrequencies), 'custom']
  .map(value => ({ value, label: t(`pelicanTests.frequency.${value}`) })))

const { data: groups, loading: groupsLoading, failed: groupsFailed, refresh: refreshGroups } =
  useKeyedResource(() => props.show && form.value.scope === 'groups' ? true : null, () => getOptions({ includeInactive: true }))

const { data: models } = useKeyedResource(
  () => {
    if (!props.show || form.value.kind !== 'iq') return null
    const id = form.value.scope === 'groups' ? form.value.group_ids[0] : form.value.provider_ids[0]
    return id === undefined ? null : { scope: form.value.scope, id }
  },
  async ({ scope, id }, signal) => scope === 'groups'
    ? await getModelsListCandidates(id, signal)
    : (await getAvailableModels(id)).map(model => model.id),
)

watch(() => props.show, open => {
  if (!open) return
  formError.value = ''
  previewNames.value = new Map()
  form.value = props.plan ? taskDraft(props.plan) : defaultDraft()
  frequency.value = frequencyForCron(form.value.cron_expression)
}, { immediate: true })

// 范围或查询条件变化时，旧的预览和仍在途的响应都作废。
watch(
  [previewKey, previewRequest],
  async ([key, request], previous, onCleanup) => {
    previewData.value = null
    previewLoading.value = false
    if (!key || request === previous?.[1]) return
    const controller = new AbortController()
    onCleanup(() => controller.abort())
    previewLoading.value = true
    try {
      const result = await previewPlan(taskDraft(form.value), controller.signal)
      if (controller.signal.aborted) return
      previewData.value = result
      previewPage.value = 1
      for (const item of result.items) {
        if (item.provider_name) previewNames.value.set(item.provider_id, item.provider_name)
      }
    } catch (error) {
      if (!controller.signal.aborted) formError.value = extractApiErrorMessage(error, t('scheduledTasks.previewFailed'))
    } finally {
      if (!controller.signal.aborted) previewLoading.value = false
    }
  },
)

function defaultDraft(): ScheduledTaskDraft {
  return {
    name: '', kind: 'relogin', scope: 'all', group_ids: [], provider_ids: [], model: '',
    cron_expression: pelicanFrequencies.every15m, enabled: false,
  }
}

function selectKind(value: unknown) {
  if ((value !== 'iq' && value !== 'relogin') || value === form.value.kind) return
  const wasDefault = frequency.value === (form.value.kind === 'iq' ? 'every6h' : 'every15m')
  form.value.kind = value
  if (value === 'iq' && form.value.scope === 'all') form.value.scope = 'groups'
  if (value === 'relogin') form.value.model = ''
  if (!props.plan && wasDefault) selectFrequency(value === 'iq' ? 'every6h' : 'every15m')
}

function selectScope(value: unknown) {
  if (value === 'groups' || value === 'providers' || (value === 'all' && form.value.kind === 'relogin')) form.value.scope = value
}

function selectFrequency(value: unknown) {
  if (value === 'custom') frequency.value = 'custom'
  else if (typeof value === 'string' && Object.prototype.hasOwnProperty.call(pelicanFrequencies, value)) {
    const preset = value as keyof typeof pelicanFrequencies
    frequency.value = preset
    form.value.cron_expression = pelicanFrequencies[preset]
  }
}

function validate(draft: ScheduledTaskDraft): boolean {
  let key = ''
  if (!draft.name || [...draft.name].length > 80 || /\p{Cc}/u.test(draft.name)) key = 'invalidName'
  else if (draft.scope === 'groups' && (!draft.group_ids.length || draft.group_ids.length > 100)) key = 'invalidGroups'
  else if (draft.scope === 'providers' && (!draft.provider_ids.length || draft.provider_ids.length > 1000)) key = 'invalidProviders'
  else if (draft.kind === 'iq' && (!draft.model || /[\s*\p{Cc}]/u.test(draft.model))) key = 'invalidModel'
  else if (!/^[\dA-Za-z*/,?\-]+(?: [\dA-Za-z*/,?\-]+){4}$/.test(draft.cron_expression)) key = 'invalidCron'
  formError.value = key ? t(`scheduledTasks.${key}`) : ''
  return !key
}

function preview() {
  if (!saving.value && !previewLoading.value && validate(taskDraft(form.value))) previewRequest.value++
}

function reasonLabel(reason: string): string {
  const key = `scheduledTasks.reason.${reason}`
  return te(key) ? t(key) : reason
}

function close() {
  if (!saving.value) emit('close')
}

async function save() {
  const draft = taskDraft(form.value)
  if (saving.value || !validate(draft)) return
  saving.value = true
  try {
    emit('saved', await savePlan(draft, props.plan?.id))
  } catch (error) {
    formError.value = extractApiErrorMessage(error, t('scheduledTasks.saveFailed'))
  } finally {
    saving.value = false
  }
}
</script>
