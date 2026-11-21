<template>
  <BaseDialog
    :show="!!plan || initialRunId !== null" width="wide" :title="t(plan ? 'scheduledTasks.history' : 'scheduledTasks.trigger.pelican')"
    :subtitle="plan?.name ?? t('scheduledTasks.runLabel', { id: initialRunId })" @close="emit('close')"
  >
    <div class="space-y-4">
      <SettingsNotice v-if="historyFailed" tone="error">
        {{ t('scheduledTasks.historyFailed') }}
        <button type="button" class="ml-2 underline" @click="refreshHistory">{{ t('common.retry') }}</button>
      </SettingsNotice>
      <ContentSkeleton v-else-if="historyLoading && !history && !selectedRun" :rows="3" />
      <p v-else-if="history && !history.length && !selectedRun" class="text-sm text-gray-600 dark:text-dark-300">
        {{ t('scheduledTasks.noHistory') }}
      </p>
      <div v-if="plan && runOptions.length" class="sm:max-w-md">
        <Select
          id="scheduled-current-run" :model-value="selectedRun" :options="runOptions"
          :aria-label="t('scheduledTasks.history')" @update:model-value="selectRun"
        />
      </div>

      <SettingsNotice v-if="runFailed" tone="error">
        {{ t('scheduledTasks.runDetailFailed') }}
        <button type="button" class="ml-2 underline" @click="refreshRun">{{ t('common.retry') }}</button>
      </SettingsNotice>
      <ContentSkeleton v-if="runLoading && !run" :rows="4" />
      <template v-if="run">
        <section class="space-y-2 rounded-surface border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800">
          <div class="flex flex-wrap items-baseline justify-between gap-2">
            <p class="flex flex-wrap items-baseline gap-2 text-sm">
              <span class="badge" :class="runBadge(run.status)">{{ t(`scheduledTasks.runStatus.${run.status}`) }}</span>
              <span class="text-gray-600 dark:text-dark-300">{{ t(`scheduledTasks.trigger.${run.trigger}`) }}</span>
              <time class="tabular-nums text-gray-600 dark:text-dark-300" :datetime="run.started_at || run.scheduled_at">
                {{ formatRunTime(run.started_at || run.scheduled_at, timezone) }}
              </time>
            </p>
            <span class="text-sm font-semibold tabular-nums">
              {{ t('scheduledTasks.progress', { completed: run.completed, total: run.total }) }}
            </span>
          </div>
          <progress
            v-if="run.total || isActiveRun(run.status)"
            class="block h-2 w-full overflow-hidden rounded-full accent-primary-600 dark:accent-primary-400"
            :max="run.total || 1" :value="run.total ? run.completed : undefined" :aria-label="t('scheduledTasks.runDetails')"
          />
          <div class="flex flex-wrap items-center justify-end gap-2 text-xs tabular-nums text-gray-600 dark:text-dark-300">
            <span>{{ t('scheduledTasks.runningCount', { count: run.running }) }}</span>
            <span>{{ t('scheduledTasks.remainingCount', { count: run.remaining }) }}</span>
          </div>
        </section>
        <p v-if="!run.items?.length" class="text-sm text-gray-600 dark:text-dark-300">
          {{ t(isActiveRun(run.status) ? 'scheduledTasks.waitingTargets' : 'scheduledTasks.emptyRun') }}
        </p>
        <div v-else class="overflow-hidden rounded-surface border border-gray-200 dark:border-dark-700">
          <ul class="divide-y divide-gray-100 dark:divide-dark-700" :aria-label="t('scheduledTasks.runDetails')">
            <li v-for="item in visibleItems" :key="item.provider_id" class="space-y-2 px-4 py-3">
              <div class="flex flex-wrap items-baseline justify-between gap-2">
                <div class="min-w-0">
                  <p class="break-words text-sm font-medium">{{ item.provider_name || `#${item.provider_id}` }}</p>
                  <p class="mt-1 break-words text-xs text-gray-500 dark:text-dark-400">
                    #{{ item.provider_id }}
                    <template v-if="item.group_id"> · {{ t('scheduledTasks.groupId', { id: item.group_id }) }}</template>
                    <code v-if="item.actual_model || item.model" class="font-mono"> · {{ item.actual_model || item.model }}</code>
                  </p>
                </div>
                <span class="badge" :class="itemBadge(item)">{{ itemLabel(item) }}</span>
              </div>
              <p v-if="item.detail || item.reason" class="whitespace-pre-wrap break-words text-xs text-gray-600 dark:text-dark-300">
                {{ detailLabel(item.detail || item.reason || '') }}
              </p>
              <details
                v-if="item.initial_answer || item.probe || item.answer || item.http_status || item.operation_id || item.replacement_id || item.finished_at"
                :open="run.trigger === 'pelican'" class="text-xs text-gray-600 dark:text-dark-300"
              >
                <summary class="w-fit cursor-pointer font-medium text-primary-700 dark:text-primary-300">{{ t('scheduledTasks.evidence') }}</summary>
                <div class="space-y-1 pt-2">
                  <div v-if="item.initial_answer">
                    <p class="font-medium">{{ t('scheduledTasks.initialAnswer') }}</p>
                    <pre class="whitespace-pre-wrap break-words font-mono">{{ item.initial_answer }}</pre>
                  </div>
                  <p v-if="item.auth_http_status && item.auth_http_status !== item.http_status">
                    {{ t('scheduledTasks.authHttpStatus', { status: item.auth_http_status }) }}
                  </p>
                  <p v-if="item.probe">
                    {{ probeLabel(item.probe.verdict) }}
                    <template v-if="item.probe.mint_status"> · {{ t('scheduledTasks.probeMintStatus', { status: item.probe.mint_status }) }}</template>
                    <template v-if="item.probe.continue_status"> · {{ t('scheduledTasks.probeContinueStatus', { status: item.probe.continue_status }) }}</template>
                  </p>
                  <p v-if="item.probe?.mint_completed !== undefined">
                    {{ t(item.probe.mint_completed ? 'scheduledTasks.initialComplete' : 'scheduledTasks.initialIncomplete') }}
                    <template v-if="item.probe.continue_status || item.probe.continue_completed">
                      · {{ t(item.probe.continue_completed ? 'scheduledTasks.continuationComplete' : 'scheduledTasks.continuationIncomplete') }}
                    </template>
                  </p>
                  <p v-if="item.probe?.reason" class="break-words">{{ detailLabel(item.probe.reason) }}</p>
                  <p v-if="item.stage === 'candy'">
                    {{ t('scheduledTasks.candyCheck') }}
                    <template v-if="item.http_status"> · {{ t('scheduledTasks.httpStatus', { status: item.http_status }) }}</template>
                    <template v-else-if="item.state === 'finished'"> · {{ t('scheduledTasks.candyNoHttpStatus') }}</template>
                  </p>
                  <p v-else-if="item.http_status">{{ t('scheduledTasks.httpStatus', { status: item.http_status }) }}</p>
                  <p v-if="item.replacement_id">{{ t('scheduledTasks.replacement', { id: item.replacement_id }) }}</p>
                  <p v-if="item.operation_id" class="break-all">{{ t('scheduledTasks.operation') }} <code class="font-mono">{{ item.operation_id }}</code></p>
                  <p v-if="item.finished_at">
                    {{ t('scheduledTasks.finishedAt') }}
                    <time class="tabular-nums" :datetime="item.finished_at">{{ formatRunTime(item.finished_at, timezone) }}</time>
                  </p>
                  <div v-if="item.candy_prompt">
                    <p class="font-medium">{{ t('scheduledTasks.candyPrompt') }}</p>
                    <pre class="whitespace-pre-wrap break-words font-mono">{{ item.candy_prompt }}</pre>
                  </div>
                  <div v-if="item.answer" class="pt-1">
                    <p class="font-medium">{{ t('scheduledTasks.answer') }}</p>
                    <pre class="mt-1 whitespace-pre-wrap break-words rounded-control bg-gray-50 p-2 font-mono dark:bg-dark-800">{{ item.answer }}</pre>
                  </div>
                </div>
              </details>
            </li>
          </ul>
          <Pagination
            v-if="run.items.length > PAGE_SIZE" v-model:page="page"
            :total="run.items.length" :page-size="PAGE_SIZE" :show-page-size-selector="false"
          />
        </div>
      </template>
    </div>
    <template #footer>
      <div class="flex justify-end gap-2">
        <button type="button" class="btn btn-secondary" :disabled="historyLoading || runLoading" @click="refresh">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': historyLoading || runLoading }" />{{ t('common.refresh') }}
        </button>
        <button type="button" class="btn btn-secondary" @click="emit('close')">{{ t('common.close') }}</button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { listRuns, readRun, type ScheduledTaskItem, type ScheduledTaskPlan } from '@/api/admin/scheduledTasks'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import Pagination from '@/components/common/Pagination.vue'
import Select from '@/components/common/Select.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import Icon from '@/components/icons/Icon.vue'
import { useKeyedResource } from '@/composables/useKeyedResource'
import { ACTIVE_RUN_POLL_MS, formatRunTime, isActiveRun, runBadge } from './taskPresentation'

const props = defineProps<{ plan: ScheduledTaskPlan | null; initialRunId: number | null; timezone: string }>()
const emit = defineEmits<{ close: []; finished: [] }>()
const { t, te } = useI18n()
const PAGE_SIZE = 16
const page = ref(1)
const selectedRun = ref<number | null>(null)
const { data: history, loading: historyLoading, failed: historyFailed, refresh: refreshHistory } =
  useKeyedResource(() => props.plan?.id ?? null, listRuns)
const { data: run, loading: runLoading, failed: runFailed, refresh: refreshRun } =
  useKeyedResource(() => selectedRun.value, readRun, result => isActiveRun(result.status) ? ACTIVE_RUN_POLL_MS : undefined)

const runOptions = computed(() => {
  const options = (history.value ?? []).map(candidate => ({
    value: candidate.id,
    label: [
      t('scheduledTasks.runLabel', { id: candidate.id }),
      formatRunTime(candidate.scheduled_at, props.timezone),
      t(`scheduledTasks.runStatus.${candidate.id === run.value?.id ? run.value.status : candidate.status}`),
    ].join(' · '),
  }))
  if (selectedRun.value !== null && !options.some(item => item.value === selectedRun.value)) {
    options.unshift({ value: selectedRun.value, label: t('scheduledTasks.runLabel', { id: selectedRun.value }) })
  }
  return options
})

const visibleItems = computed(() => {
  const offset = (page.value - 1) * PAGE_SIZE
  return run.value?.items?.slice(offset, offset + PAGE_SIZE) ?? []
})

watch([() => props.plan?.id, () => props.initialRunId], ([, initial]) => {
  selectedRun.value = initial
}, { immediate: true })

watch(history, items => {
  if (selectedRun.value === null && items?.length) selectedRun.value = items[0].id
})

watch(selectedRun, () => { page.value = 1 })

watch(run, (current, previous) => {
  if (current && current.id === previous?.id && isActiveRun(previous.status) && !isActiveRun(current.status)) {
    refreshHistory()
    emit('finished')
  }
})

function selectRun(value: unknown) {
  if (typeof value === 'number') selectedRun.value = value
}

function refresh() {
  refreshHistory()
  refreshRun()
}

function itemLabel(item: ScheduledTaskItem): string {
  if (item.outcome) {
    const key = `scheduledTasks.outcome.${item.outcome}`
    return t(te(key) ? key : 'scheduledTasks.unsupportedResult')
  }
  if (isActiveRun(item.state) && item.stage && (item.state === 'running' || item.operation_id)) {
    const key = `scheduledTasks.stage.${item.stage}`
    if (te(key)) return t(key)
  }
  return t(`scheduledTasks.runStatus.${item.state}`)
}

function detailLabel(detail: string): string {
  const key = `scheduledTasks.reason.${detail}`
  return te(key) ? t(key) : detail
}

function probeLabel(verdict: string): string {
  const key = `scheduledTasks.probe.${verdict}`
  return t(te(key) ? key : 'scheduledTasks.probe.inconclusive')
}

function itemBadge(item: ScheduledTaskItem) {
  switch (item.outcome) {
    case 'auth_recovered':
    case 'relogin_succeeded':
    case 'stable':
    case 'candy_passed':
      return 'badge-success'
    case 'needs_action':
    case 'inconclusive':
      return 'badge-warning'
    case 'suspended':
    case 'failed':
      return 'badge-danger'
    default:
      return isActiveRun(item.state) ? 'badge-primary' : 'badge-gray'
  }
}
</script>
