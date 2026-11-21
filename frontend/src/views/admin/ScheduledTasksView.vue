<template>
  <AppLayout>
    <template #page-heading-meta>
      <p v-if="timezone" class="flex items-center gap-2 text-xs text-gray-500 dark:text-dark-400">
        <Icon name="clock" size="sm" />{{ t('scheduledTasks.timezone', { timezone }) }}
      </p>
    </template>
    <template #page-heading-actions>
      <div class="flex flex-wrap items-center gap-2">
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="refresh">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />{{ t('common.refresh') }}
        </button>
        <button type="button" class="btn btn-primary" data-testid="scheduled-create" @click="openEditor(null)">
          <Icon name="plus" size="sm" />{{ t('scheduledTasks.createPlan') }}
        </button>
      </div>
    </template>

    <div class="space-y-4">
      <SettingsNotice v-if="failed" tone="error">{{ t('scheduledTasks.loadFailed') }}</SettingsNotice>
      <SettingsNotice v-if="actionError" tone="error">{{ actionError }}</SettingsNotice>

      <DataTable :columns="columns" :data="plans" :loading="loading && !listData" :expandable-actions="false" row-key="id">
        <template #empty>
          <EmptyState :title="t('scheduledTasks.noPlans')" :description="t('scheduledTasks.noPlansHint')" />
        </template>
        <template #cell-name="{ row }">
          <p class="break-words font-semibold">{{ row.name }}</p>
          <span class="text-xs text-gray-500 dark:text-dark-400">{{ t(`scheduledTasks.kind.${row.kind}`) }}</span>
        </template>
        <template #cell-scope="{ row }">
          <p>{{ scopeLabel(row) }}</p>
          <code v-if="row.model" class="break-all font-mono text-xs text-gray-500 dark:text-dark-400">{{ row.model }}</code>
        </template>
        <template #cell-cron_expression="{ value }">
          <span v-if="frequencyForCron(value) !== 'custom'">{{ t(`pelicanTests.frequency.${frequencyForCron(value)}`) }}</span>
          <code v-else class="font-mono text-xs">{{ value }}</code>
        </template>
        <template #cell-enabled="{ row }">
          <div class="flex flex-wrap items-center gap-2">
            <Toggle
              size="sm" :model-value="row.enabled" :disabled="busy.has(row.id)"
              :aria-label="t('scheduledTasks.scheduleFor', { name: row.name })"
              @update:model-value="setEnabled(row, $event)"
            />
            <span class="text-xs">{{ t(row.enabled ? 'scheduledTasks.enabled' : 'scheduledTasks.paused') }}</span>
          </div>
        </template>
        <template #cell-last_run_at="{ row }">
          <p v-if="row.last_run_status">
            <button type="button" class="badge" :class="runBadge(row.last_run_status)" @click="openHistory(row, row.last_run_id ?? null)">
              {{ t(`scheduledTasks.runStatus.${row.last_run_status}`) }}
            </button>
          </p>
          <time v-if="row.last_run_at" class="text-xs tabular-nums text-gray-600 dark:text-dark-300" :datetime="row.last_run_at">
            {{ formatRunTime(row.last_run_at, timezone) }}
          </time>
          <span v-else-if="!row.last_run_status" class="text-xs text-gray-500 dark:text-dark-400">{{ t('scheduledTasks.neverRun') }}</span>
        </template>
        <template #cell-next_run_at="{ row }">
          <time v-if="row.enabled && row.next_run_at" class="text-xs tabular-nums" :datetime="row.next_run_at">
            {{ formatRunTime(row.next_run_at, timezone) }}
          </time>
          <span v-else class="text-gray-500 dark:text-dark-400">—</span>
        </template>
        <template #cell-actions="{ row }">
          <div class="flex flex-wrap items-center justify-end gap-2">
            <button
              type="button" class="btn btn-secondary btn-sm" :data-testid="`scheduled-run-${row.id}`" :disabled="busy.has(row.id)"
              @click="handleRun(row)"
            >
              <Icon :name="isActiveRun(row.last_run_status) ? 'clock' : 'play'" size="sm" />
              {{ t(isActiveRun(row.last_run_status) ? 'scheduledTasks.runDetails' : 'scheduledTasks.runNow') }}
            </button>
            <button
              type="button" class="btn-icon-sm text-primary-700 hover:bg-primary-50 dark:text-primary-300 dark:hover:bg-dark-700"
              :title="t('scheduledTasks.history')" :aria-label="t('scheduledTasks.history')" @click="openHistory(row)"
            ><Icon name="history" size="sm" /></button>
            <button
              type="button" class="btn-icon-sm text-primary-700 hover:bg-primary-50 dark:text-primary-300 dark:hover:bg-dark-700"
              :disabled="busy.has(row.id)" :title="t('common.edit')" :aria-label="t('common.edit')" @click="openEditor(row)"
            ><Icon name="edit" size="sm" /></button>
          </div>
        </template>
      </DataTable>
    </div>

    <TaskPlanEditor :show="editorOpen" :plan="editing" :timezone="timezone" @close="editorOpen = false" @saved="saved" />
    <TaskRunDialog :plan="historyPlan" :initial-run-id="initialRunId" :timezone="timezone" @close="closeHistory" @finished="refresh" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { listPlans, runPlan, savePlan, type ScheduledTaskPlan } from '@/api/admin/scheduledTasks'
import TaskPlanEditor from '@/components/admin/scheduledTasks/TaskPlanEditor.vue'
import TaskRunDialog from '@/components/admin/scheduledTasks/TaskRunDialog.vue'
import { ACTIVE_RUN_POLL_MS, formatRunTime, isActiveRun, runBadge, taskDraft } from '@/components/admin/scheduledTasks/taskPresentation'
import DataTable from '@/components/common/DataTable.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import { frequencyForCron } from '@/components/pelican/pelicanFrequency'
import { useKeyedResource } from '@/composables/useKeyedResource'
import { extractApiErrorMessage } from '@/utils/apiError'

const { t } = useI18n()
const { data: listData, loading, failed, refresh } = useKeyedResource(
  () => true, (_key, signal) => listPlans(signal), result => {
    if (result.items.some(plan => isActiveRun(plan.last_run_status))) return ACTIVE_RUN_POLL_MS
    if (result.items.some(plan => plan.enabled)) return 30_000
  },
)
const plans = computed(() => listData.value?.items ?? [])
const timezone = computed(() => listData.value?.timezone ?? '')
const columns = computed(() => [
  { key: 'name', label: t('scheduledTasks.fields.name') },
  { key: 'scope', label: t('scheduledTasks.fields.scope') },
  { key: 'cron_expression', label: t('scheduledTasks.fields.frequency') },
  { key: 'enabled', label: t('scheduledTasks.fields.status') },
  { key: 'last_run_at', label: t('scheduledTasks.fields.lastRun') },
  { key: 'next_run_at', label: t('scheduledTasks.fields.nextRun') },
  { key: 'actions', label: t('common.actions') },
])
const busy = ref(new Set<number>())
const actionError = ref('')
const editorOpen = ref(false)
const editing = ref<ScheduledTaskPlan | null>(null)
const historyPlan = ref<ScheduledTaskPlan | null>(null)
const initialRunId = ref<number | null>(null)

function scopeLabel(plan: ScheduledTaskPlan) {
  if (plan.scope === 'all') return t('scheduledTasks.scope.all')
  const count = plan.scope === 'groups' ? plan.group_ids?.length : plan.provider_ids?.length
  return t(`scheduledTasks.scopeCount.${plan.scope}`, { count: count ?? 0 })
}

function openEditor(plan: ScheduledTaskPlan | null) {
  actionError.value = ''
  editing.value = plan
  editorOpen.value = true
}

function saved(plan: ScheduledTaskPlan) {
  editorOpen.value = false
  replacePlan(plan)
  refresh()
}

function replacePlan(plan: ScheduledTaskPlan) {
  if (!listData.value) return
  const items = listData.value.items.some(item => item.id === plan.id)
    ? listData.value.items.map(item => item.id === plan.id ? plan : item)
    : [plan, ...listData.value.items]
  listData.value = { ...listData.value, items }
}

function openHistory(plan: ScheduledTaskPlan, runId: number | null = null) {
  initialRunId.value = runId
  historyPlan.value = plan
}

function closeHistory() {
  historyPlan.value = null
  initialRunId.value = null
}

async function setEnabled(plan: ScheduledTaskPlan, enabled: boolean) {
  if (busy.value.has(plan.id) || plan.enabled === enabled) return
  busy.value.add(plan.id)
  actionError.value = ''
  try {
    replacePlan(await savePlan(taskDraft({ ...plan, enabled }), plan.id))
  } catch (error) {
    actionError.value = extractApiErrorMessage(error, t('scheduledTasks.saveFailed'))
  } finally {
    busy.value.delete(plan.id)
    refresh()
  }
}

async function handleRun(plan: ScheduledTaskPlan) {
  if (busy.value.has(plan.id)) return
  if (isActiveRun(plan.last_run_status)) {
    openHistory(plan, plan.last_run_id ?? null)
    return
  }
  busy.value.add(plan.id)
  actionError.value = ''
  try {
    const runId = await runPlan(plan.id)
    // 立即显示本轮编号，后续读取填入真实进度。
    replacePlan({ ...plan, last_run_id: runId, last_run_status: 'queued' })
    openHistory(plan, runId)
  } catch (error) {
    actionError.value = extractApiErrorMessage(error, t('scheduledTasks.runFailed'))
  } finally {
    busy.value.delete(plan.id)
    refresh()
  }
}
</script>
