<template>
  <AppLayout>
    <template #page-heading-actions>
      <div class="flex flex-wrap items-center gap-2">
        <button type="button" class="btn btn-secondary" :disabled="loading" @click="refreshPage">
          <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />{{ t('common.refresh') }}
        </button>
        <RouterLink v-if="showcaseEnabled && pelicanVisibility?.enabled !== false" to="/pelican-showcase" class="btn btn-secondary" data-testid="view-showcase">
          <Icon name="eye" size="sm" />{{ t('pelicanTests.showcase.view') }}
        </RouterLink>
        <button type="button" class="btn btn-primary" data-testid="create-plan" @click="openEditor(null)">
          <Icon name="plus" size="sm" />{{ t('pelicanTests.createPlan') }}
        </button>
      </div>
    </template>

    <div class="space-y-4">
      <SettingsNotice v-if="failed" tone="error">{{ t('pelicanTests.plansLoadFailed') }}</SettingsNotice>
      <SettingsNotice v-if="actionError && !deleting" tone="error">{{ t(actionError) }}</SettingsNotice>
      <div class="grid min-w-0 items-start gap-4 xl:grid-cols-[minmax(19rem,1fr)_minmax(0,2fr)]">
        <PelicanShowcaseSettings :key="pelicanViewerId || 0" ref="settingsCard" @configured="showcaseEnabled = $event" />
        <section class="card min-w-0">
        <header class="card-header">
          <h2 class="text-base font-semibold text-gray-900 dark:text-dark-50">{{ t('pelicanTests.plans') }}</h2>
        </header>
        <ContentSkeleton v-if="loading && !plans" class="p-6" />
        <EmptyState v-else-if="plans && !plans.length" :title="t('pelicanTests.noPlans')" :description="t('pelicanTests.noPlansHint')" />
        <ul v-else class="divide-y divide-gray-200 dark:divide-dark-700">
          <li v-for="plan in plans" :key="plan.id" class="grid items-center gap-4 p-4 md:grid-cols-2 xl:grid-cols-2 2xl:grid-cols-3">
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <ModelIcon :model="plan.model_id" class="shrink-0" />
                <h3 class="break-all font-mono text-sm font-semibold text-gray-900 dark:text-dark-50">{{ plan.model_id }}</h3>
              </div>
              <p class="mt-1 break-words text-sm text-gray-600 dark:text-dark-300">
                {{ plan.group_name }} · {{ plan.provider_name || (plan.provider_id ? '#' + plan.provider_id : t('pelicanTests.autoProvider')) }}
              </p>
              <p class="mt-1 break-words text-xs text-gray-500 dark:text-dark-400">
                <template v-if="overriddenQuestionName(plan)">{{ plan.question_name }} · </template>{{ t(`pelicanTests.kind.${plan.question_kind}`) }}
                <template v-if="plan.question_kind === 'pelican' && !plan.question_prompt"> · {{ t('pelicanTests.rotatingQuestion') }}</template>
                <template v-if="plan.question_kind !== 'state_probe'"> · {{ t('pelicanTests.configuredEffort') }} {{ t(effortLabel(plan.reasoning_effort || 'default')) }}</template>
              </p>
            </div>
            <div class="space-y-1 text-xs text-gray-600 dark:text-dark-300">
              <div class="flex flex-wrap items-center gap-2">
                <Toggle
                  size="sm" :model-value="plan.enabled" :data-testid="`plan-schedule-${plan.id}`" :aria-label="t('pelicanTests.scheduleFor', { model: plan.model_id })"
                  :disabled="busy.has(plan.id) || plan.run_state !== 'idle'"
                  @update:model-value="perform(plan, 'toggle')"
                />
                <span>{{ t(plan.enabled ? 'pelicanTests.scheduled' : 'pelicanTests.paused') }}</span>
                <code v-if="frequencyForCron(plan.cron_expression) === 'custom'" class="font-mono">{{ plan.cron_expression }}</code>
                <span v-else>{{ t(`pelicanTests.frequency.${frequencyForCron(plan.cron_expression)}`) }}</span>
              </div>
              <p v-if="plan.next_run_at">
                {{ t('pelicanTests.nextRun') }} <time :datetime="plan.next_run_at">{{ formatDateTimeToMinute(plan.next_run_at) }}</time>
              </p>
              <p v-if="plan.last_run_at">
                {{ t('pelicanTests.lastRun') }} <time :datetime="plan.last_run_at">{{ formatDateTimeToMinute(plan.last_run_at) }}</time>
              </p>
            </div>
            <div class="flex flex-wrap items-center gap-2 md:col-span-2 2xl:col-span-1 2xl:justify-end">
              <button
                type="button" class="btn btn-secondary btn-sm" :data-testid="`run-plan-${plan.id}`"
                :disabled="busy.has(plan.id) || plan.run_state !== 'idle'" @click="perform(plan, 'run')"
              >
                <template v-if="busy.get(plan.id) === 'run'">
                  <Icon name="loader" size="sm" class="animate-spin" />{{ t('common.processing') }}
                </template>
                <template v-else>
                  <Icon :name="plan.run_state === 'idle' ? 'play' : 'clock'" size="sm" />
                  {{ t(plan.run_state === 'idle' ? 'pelicanTests.runNow' : `pelicanTests.runState.${plan.run_state}`) }}
                </template>
              </button>
              <button type="button" class="btn btn-secondary btn-sm" @click="historyPlanId = plan.id">
                <Icon name="history" size="sm" />{{ t('pelicanTests.history') }}
              </button>
              <button
                type="button" class="btn-icon-sm text-primary-700 hover:bg-primary-50 dark:text-primary-300 dark:hover:bg-dark-700"
                :aria-label="t('common.edit')" :title="t('common.edit')" :data-testid="`edit-plan-${plan.id}`"
                :disabled="busy.has(plan.id) || plan.run_state !== 'idle'" @click="openEditor(plan)"
              ><Icon name="edit" size="sm" /></button>
              <button
                type="button" class="btn-icon-sm text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-900/20"
                :aria-label="t('common.delete')" :title="t('common.delete')" :data-testid="`delete-plan-${plan.id}`"
                :disabled="busy.has(plan.id)" @click="confirmDelete(plan)"
              ><Icon name="trash" size="sm" /></button>
            </div>
          </li>
        </ul>
        </section>
      </div>
    </div>

    <PelicanPlanEditor :show="editorOpen" :plan="editing" :plans="plans || []" :target="planTarget" @saved="saved" @close="editorOpen = false" />
    <ConfirmDialog
      :show="!!deleting" :title="t('pelicanTests.deletePlan')" :confirm-text="t('common.delete')" danger
      :message="t('pelicanTests.deleteConfirm', { model: deleting?.model_id, group: deleting?.group_name })"
      :loading="!!deleting && busy.has(deleting.id)" @cancel="closeDelete" @confirm="deleting && perform(deleting, 'delete')"
    >
      <SettingsNotice v-if="actionError" tone="error">{{ t(actionError) }}</SettingsNotice>
    </ConfirmDialog>

    <BaseDialog
      :show="historyPlanId !== null" width="wide" :title="t('pelicanTests.history')"
      :subtitle="historyPlan && `${historyPlan.group_name} · ${historyPlan.model_id}`" @close="closeHistory"
    >
      <div class="space-y-4">
        <SettingsNotice v-if="historyPlan && historyPlan.run_state !== 'idle'">
          {{ t(`pelicanTests.runState.${historyPlan.run_state}`) }} · {{ t('pelicanTests.waitingForResult') }}
        </SettingsNotice>
        <SettingsNotice v-if="historyFailed" tone="error">{{ t('pelicanTests.historyLoadFailed') }}</SettingsNotice>
        <ContentSkeleton v-if="historyLoading && !history" />
        <EmptyState v-else-if="history && !history.length && historyPlan?.run_state === 'idle'" :title="t('pelicanTests.noHistory')" />
        <ul v-else class="divide-y divide-gray-200 dark:divide-dark-700">
          <li v-for="result in history" :key="result.id" class="flex flex-wrap items-center justify-between gap-4 py-3">
            <div class="min-w-0 space-y-1 text-sm">
              <div class="flex flex-wrap items-center gap-2">
                <PelicanStatus :status="result.status" :probe="result.question_kind === 'state_probe'" />
                <time :datetime="result.tested_at">{{ formatDateTime(result.tested_at) }}</time>
              </div>
              <p class="break-all font-mono">{{ result.model_id }}</p>
              <p class="text-xs text-gray-500 dark:text-dark-400">
                {{ overriddenQuestionName(result) || t(`pelicanTests.kind.${result.question_kind}`) }} · {{ t('pelicanTests.latency', { ms: result.latency_ms }) }}
                <template v-if="result.question_kind !== 'state_probe'"> · {{ t('pelicanTests.reasoningEffort') }} {{ t(effortLabel(result.reasoning_effort)) }}</template>
                <template v-if="result.provider_name"> · {{ result.provider_name }}</template>
              </p>
              <p v-if="result.schedule_action" class="text-xs text-gray-600 dark:text-dark-300">
                {{ t(`pelicanTests.candySuspension.${result.schedule_action}`) }}
              </p>
              <p v-if="result.detail_code === 'candy_exhausted' || result.detail_code === 'candy_busy'" class="text-xs text-gray-600 dark:text-dark-300">
                {{ t(`pelicanTests.reason.${result.detail_code}`) }}
              </p>
            </div>
            <button type="button" class="btn btn-secondary btn-sm" :data-testid="`open-sample-${result.id}`" @click="sampleId = result.id">
              <Icon name="eye" size="sm" />{{ t('pelicanTests.openSample') }}
            </button>
          </li>
        </ul>
      </div>
      <template #footer>
        <div class="flex justify-end gap-2">
          <button type="button" class="btn btn-secondary" :disabled="historyLoading" @click="refreshHistory">
            <Icon name="refresh" size="sm" :class="{ 'animate-spin': historyLoading }" />{{ t('common.refresh') }}
          </button>
          <button type="button" class="btn btn-secondary" @click="closeHistory">{{ t('common.close') }}</button>
        </div>
      </template>
    </BaseDialog>
    <PelicanResultDialog :result-id="sampleId" :load-result="getResult" :review="reviewResult" @reviewed="reviewed" @close="sampleId = null" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { LocationQuery } from 'vue-router'
import { deletePlan, getResult, listPlans, listResults, reviewResult, runPlan, updatePlan, type PelicanPlan, type PelicanPlanTarget } from '@/api/admin/pelicanTests'
import { pelicanEfforts, pelicanPlanKinds, type PelicanReviewStatus } from '@/api/pelicanTests'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import PelicanPlanEditor from '@/components/pelican/PelicanPlanEditor.vue'
import PelicanResultDialog from '@/components/pelican/PelicanResultDialog.vue'
import PelicanStatus from '@/components/pelican/PelicanStatus.vue'
import PelicanShowcaseSettings from '@/components/pelican/PelicanShowcaseSettings.vue'
import { pelicanViewerId, pelicanVisibility } from '@/composables/usePelicanVisibility'
import { effortLabel } from '@/components/pelican/pelicanArtwork'
import { frequencyForCron } from '@/components/pelican/pelicanFrequency'
import { pelicanPollDelay, usePelicanResource } from '@/components/pelican/usePelicanResource'
import { overriddenQuestionName } from '@/i18n/pelicanQuestionNames'
import { formatDateTime, formatDateTimeToMinute } from '@/utils/format'

const props = defineProps<{ scheduleQuery?: LocationQuery }>()
const { t } = useI18n()
const { data: plans, loading, failed, refresh: refreshPlans } = usePelicanResource(
  () => true, (_key, signal) => listPlans(signal), pelicanPollDelay,
)
const settingsCard = ref<InstanceType<typeof PelicanShowcaseSettings> | null>(null)
const showcaseEnabled = ref(false)

function refreshPage() {
  refreshPlans()
  void settingsCard.value?.refresh()
}

type PlanAction = 'run' | 'toggle' | 'delete'
const busy = ref(new Map<number, PlanAction>())
const actionError = ref('')
const editing = ref<PelicanPlan | null>(null)
const editorOpen = ref(false)
const planTarget = ref<PelicanPlanTarget | null>(null)
const deleting = ref<PelicanPlan | null>(null)
const historyPlanId = ref<number | null>(null)
const sampleId = ref<number | null>(null)
const historyPlan = computed(() => plans.value?.find(plan => plan.id === historyPlanId.value))
const { data: history, loading: historyLoading, failed: historyFailed, refresh: refreshHistory } =
  usePelicanResource(() => historyPlanId.value, listResults)

// 已打开的运行记录跟随计划进度更新。刚选中计划时，由计划编号触发首次读取。
watch([() => historyPlan.value?.id, () => historyPlan.value?.run_state, () => historyPlan.value?.last_run_at],
  ([id], [previousId]) => {
    if (id && id === previousId) refreshHistory()
    if (!id && historyPlanId.value !== null && plans.value) closeHistory()
  })

// 智商测试的链接指向一个账号。分组成员关系在打开编辑器后读取。
watch(() => props.scheduleQuery, query => {
  const raw = query?.provider_id
  if (!query || typeof raw !== 'string' || !/^[1-9]\d*$/.test(raw) || !Number.isSafeInteger(Number(raw))) return
  const model = query.model_id
  openEditor(null, {
    provider_id: Number(raw),
    model_id: typeof model === 'string' && !/[\s*\p{Cc}]/u.test(model) && new TextEncoder().encode(model).length <= 128 ? model : '',
    question_kind: pelicanPlanKinds.find(kind => kind === query.question_kind) ?? 'pelican',
    reasoning_effort: query.question_kind === 'state_probe' ? '' : pelicanEfforts.find(effort => effort === query.reasoning_effort) ?? '',
  })
}, { immediate: true })

function openEditor(plan: PelicanPlan | null, target: PelicanPlanTarget | null = null) {
  planTarget.value = target
  editing.value = plan
  editorOpen.value = true
}

function saved() {
  editorOpen.value = false
  refreshPlans()
}

function confirmDelete(plan: PelicanPlan) {
  actionError.value = ''
  deleting.value = plan
}

function closeDelete() {
  if (deleting.value && !busy.value.has(deleting.value.id)) {
    deleting.value = null
    actionError.value = ''
  }
}

function closeHistory() {
  sampleId.value = null
  historyPlanId.value = null
}

function reviewed(id: number, status: PelicanReviewStatus) {
  if (history.value) history.value = history.value.map(result => result.id === id ? { ...result, status } : result)
}

async function perform(plan: PelicanPlan, action: PlanAction) {
  if (busy.value.has(plan.id) || (action !== 'delete' && plan.run_state !== 'idle')) return
  busy.value.set(plan.id, action)
  actionError.value = ''
  try {
    if (action === 'run') {
      await runPlan(plan.id)
      // 排队响应到达后就锁定运行按钮，下一次读取会返回服务端的当前进度。
      plans.value = (plans.value || []).map(item => item.id === plan.id ? { ...item, run_state: 'queued' } : item)
      historyPlanId.value = plan.id
    } else if (action === 'toggle') {
      const updated = await updatePlan(plan.id, { ...plan, enabled: !plan.enabled })
      if (plans.value) plans.value = plans.value.map(item => item.id === plan.id ? updated : item)
    } else {
      await deletePlan(plan.id)
      if (plans.value) plans.value = plans.value.filter(item => item.id !== plan.id)
      deleting.value = null
      if (historyPlanId.value === plan.id) closeHistory()
    }
  } catch {
    actionError.value = `pelicanTests.actionFailed.${action}`
  } finally {
    busy.value.delete(plan.id)
    refreshPlans()
  }
}
</script>
