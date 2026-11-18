<template>
  <article class="min-w-0 space-y-4 rounded-surface border border-gray-200 p-4 dark:border-dark-600" data-testid="iq-probe-result">
    <header class="flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-dark-50">{{ t('providerIQ.probe.observation') }}</h3>
      <PelicanStatus :status="stateProbeStatus[result.verdict]" probe />
    </header>
    <dl class="grid grid-cols-1 gap-4 text-sm sm:grid-cols-2">
      <div v-for="item in models" :key="item.label" class="min-w-0">
        <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t(item.label) }}</dt>
        <dd class="mt-1 break-all font-mono">{{ item.value || t('providerIQ.probe.unobserved') }}</dd>
      </div>
      <div>
        <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('providerIQ.probe.newState') }}</dt>
        <dd class="mt-1" data-testid="iq-probe-new-state">{{ t(newStateLabel) }}</dd>
      </div>
      <div>
        <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('providerIQ.probe.finishedAt') }}</dt>
        <dd class="mt-1 tabular-nums"><time :datetime="result.finished_at">{{ formatDateTime(result.finished_at) }}</time></dd>
      </div>
      <div>
        <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('providerIQ.probe.latency') }}</dt>
        <dd class="mt-1 tabular-nums">{{ t('pelicanTests.latency', { ms: result.latency_ms }) }}</dd>
      </div>
    </dl>
    <div class="grid gap-4 sm:grid-cols-2">
      <section v-for="step in steps" :key="step.key" class="min-w-0 rounded-control bg-gray-50 p-3 text-sm dark:bg-dark-800">
        <h4 class="font-medium">{{ t('providerIQ.probe.' + step.key) }}</h4>
        <dl class="mt-2 space-y-2">
          <div class="flex flex-wrap items-baseline justify-between gap-x-2">
            <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('providerIQ.probe.httpStatus') }}</dt>
            <dd class="tabular-nums" :data-testid="'iq-probe-' + step.key + '-status'">{{ step.status || t('providerIQ.probe.noResponse') }}</dd>
          </div>
          <div class="flex flex-wrap items-baseline justify-between gap-x-2">
            <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('providerIQ.probe.completion') }}</dt>
            <dd :data-testid="'iq-probe-' + step.key + '-complete'">{{ t(stepCompletion(step)) }}</dd>
          </div>
          <div class="flex flex-wrap items-baseline justify-between gap-x-2">
            <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('providerIQ.probe.stepTime') }}</dt>
            <dd class="tabular-nums" :data-testid="'iq-probe-' + step.key + '-duration'">{{ step.ms != null && (step.status || step.ms) ? t('pelicanTests.latency', { ms: step.ms }) : t('providerIQ.probe.unobserved') }}</dd>
          </div>
          <div v-if="step.reported && result.mint_reported_model !== result.continue_reported_model" class="space-y-1">
            <dt class="text-xs text-gray-500 dark:text-dark-400">{{ t('providerIQ.probe.reportedModel') }}</dt>
            <dd class="break-all font-mono text-xs">{{ step.reported }}</dd>
          </div>
        </dl>
      </section>
    </div>
    <SettingsNotice v-if="result.failure" tone="warning">
      <p class="font-medium">{{ t('providerIQ.probe.failureLabel') }}</p>
      <p>{{ t(result.failure === 'unknown' ? 'providerIQ.probe.unknownFailure' : 'pelicanTests.probe.reason.' + result.failure) }}</p>
    </SettingsNotice>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { stateProbeStatus, type IQStateProbeResult } from '@/api/admin/providerIQ'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import PelicanStatus from '@/components/pelican/PelicanStatus.vue'
import { formatDateTime } from '@/utils/format'

const props = defineProps<{ result: IQStateProbeResult }>()
const { t } = useI18n()
const models = computed(() => [
  { label: 'providerIQ.model', value: props.result.requested_model },
  { label: 'providerIQ.probe.sentModel', value: props.result.model },
  { label: 'providerIQ.probe.reportedModel', value: props.result.reported_model },
])
const steps = computed(() => [
  { key: 'first', status: props.result.mint_status, complete: props.result.mint_completed, ms: props.result.mint_latency_ms, reported: props.result.mint_reported_model },
  { key: 'next', status: props.result.continue_status, complete: props.result.continue_completed, ms: props.result.continue_latency_ms, reported: props.result.continue_reported_model },
])
function stepCompletion(step: { status: number; complete?: boolean }) {
  if (!step.status || step.complete == null) return 'providerIQ.probe.unobserved'
  return step.complete ? 'providerIQ.probe.received' : 'providerIQ.probe.notReceived'
}
// 续接中断时 new_ticket 也是 false，需要完整观察才能显示“否”。
const newStateLabel = computed(() => {
  if (props.result.verdict === 'inconclusive') return 'providerIQ.probe.unobserved'
  return props.result.new_ticket ? 'providerIQ.probe.yes' : 'providerIQ.probe.no'
})
</script>
