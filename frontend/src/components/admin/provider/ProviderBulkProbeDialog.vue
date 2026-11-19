<template>
  <BaseDialog
    :show="true"
    :title="t('admin.providers.bulkProbe.title')"
    :subtitle="t('admin.providers.bulkProbe.modelHint', { model: stateProbeDefaultModel })"
    width="wide"
    :body-scroll="false"
    @close="close"
  >
    <div class="flex min-h-0 flex-col gap-3">
      <div class="shrink-0 space-y-2">
        <p class="text-sm font-medium tabular-nums text-gray-900 dark:text-dark-50" aria-live="polite">
          {{ progressLabel }}
        </p>
        <div
          class="h-1 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700"
          role="progressbar"
          :aria-label="t('admin.providers.bulkProbe.title')"
          :aria-valuemin="0"
          :aria-valuemax="rows.length"
          :aria-valuenow="processed"
          :aria-valuetext="progressLabel"
        >
          <div class="h-full rounded-full bg-primary-500" :style="{ width: progressWidth }"></div>
        </div>
        <ul v-if="summary.length" class="flex flex-wrap gap-x-3 gap-y-1 text-xs text-gray-600 dark:text-dark-300">
          <li v-for="{ state, total } in summary" :key="state">
            {{ t('admin.providers.bulkProbe.count', { state: stateLabel(state), count: total }) }}
          </li>
        </ul>
      </div>

      <div class="flex min-h-0 flex-col overflow-hidden rounded-surface border border-gray-200 dark:border-dark-600">
        <ul :key="page" class="min-h-0 divide-y divide-gray-200 overflow-auto overscroll-contain dark:divide-dark-700">
          <li v-for="row in visibleRows" :key="row.id" :data-testid="`provider-bulk-probe-row-${row.id}`">
            <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2 px-4 py-3 text-sm">
              <span class="min-w-0 flex-1 break-words font-medium text-gray-900 dark:text-dark-50">
                {{ row.name || `#${row.id}` }}
              </span>
              <div class="flex flex-wrap items-center gap-2">
                <template v-if="row.result">
                  <PelicanStatus :status="stateProbeStatus[row.result.verdict]" probe />
                  <span class="whitespace-nowrap text-xs tabular-nums text-gray-600 dark:text-dark-300">
                    {{ t('pelicanTests.latency', { ms: row.result.latency_ms }) }}
                  </span>
                  <button
                    type="button"
                    class="btn btn-ghost btn-sm"
                    :aria-expanded="expandedId === row.id"
                    :aria-controls="`provider-bulk-probe-detail-${row.id}`"
                    :aria-label="t(expandedId === row.id ? 'admin.providers.bulkProbe.hideDetailsFor' : 'admin.providers.bulkProbe.detailsFor', { provider: row.name || `#${row.id}` })"
                    @click="expandedId = expandedId === row.id ? null : row.id"
                  >
                    {{ t(expandedId === row.id ? 'admin.providers.bulkProbe.hideDetails' : 'admin.providers.bulkProbe.details') }}
                  </button>
                </template>
                <span v-else class="badge whitespace-nowrap" :class="stateTone[row.state] ?? 'badge-gray'">
                  {{ stateLabel(row.state) }}
                </span>
              </div>
            </div>
            <div v-if="row.result" :id="`provider-bulk-probe-detail-${row.id}`" :hidden="expandedId !== row.id" class="px-4 pb-3">
              <ProviderIQProbeResult v-if="expandedId === row.id" :result="row.result" />
            </div>
          </li>
        </ul>
        <Pagination
          v-if="rows.length > RESULTS_PAGE_SIZE"
          v-model:page="page"
          :total="rows.length"
          :page-size="RESULTS_PAGE_SIZE"
          :show-page-size-selector="false"
        />
      </div>
    </div>
    <template #footer>
      <button
        v-if="pending > 0"
        type="button"
        class="btn btn-secondary"
        data-testid="provider-bulk-probe-stop"
        @click="stop"
      >
        {{ t('admin.providers.bulkProbe.stop') }}
      </button>
      <button type="button" class="btn btn-secondary" @click="close">{{ t('common.close') }}</button>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { runIQStateProbe, stateProbeDefaultModel, stateProbeStatus, type IQStateProbeResult } from '@/api/admin/providerIQ'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Pagination from '@/components/common/Pagination.vue'
import PelicanStatus from '@/components/pelican/PelicanStatus.vue'
import type { Provider } from '@/types'
import { extractApiErrorCode } from '@/utils/apiError'
import ProviderIQProbeResult from './ProviderIQProbeResult.vue'

const PROBE_CONCURRENCY = 3
const RESULTS_PAGE_SIZE = 50
const probeStates = ['queued', 'running', 'healthy', 'degraded', 'inconclusive', 'unsupported', 'busy', 'missing', 'failed', 'cancelled'] as const
type ProbeState = typeof probeStates[number]
type ProbeErrorState = Extract<ProbeState, 'unsupported' | 'busy' | 'missing' | 'failed'>

interface ProbeRow {
  id: number
  name: string
  state: ProbeState
  result: IQStateProbeResult | null
}

const stateTone: Partial<Record<ProbeState, string>> = {
  running: 'badge-primary',
  busy: 'badge-warning',
  failed: 'badge-danger'
}
const stateByErrorCode = new Map<string, ProbeErrorState>([
  ['IQ_TEST_UNSUPPORTED', 'unsupported'],
  ['IQ_TEST_BUSY', 'busy'],
  ['IQ_TEST_NOT_FOUND', 'missing'],
])

const props = defineProps<{ providers: Pick<Provider, 'id' | 'name'>[] }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()

const controller = new AbortController()
const rows = reactive<ProbeRow[]>(props.providers.map(({ id, name }) => ({ id, name, state: 'queued', result: null })))
const page = ref(1)
const expandedId = ref<number | null>(null)
const visibleRows = computed(() => rows.slice((page.value - 1) * RESULTS_PAGE_SIZE, page.value * RESULTS_PAGE_SIZE))
const stateCounts = computed(() => {
  const counts = new Map<ProbeState, number>()
  for (const { state } of rows) counts.set(state, (counts.get(state) ?? 0) + 1)
  return counts
})
const count = (state: ProbeState) => stateCounts.value.get(state) ?? 0
const summary = computed(() => probeStates.map(state => ({ state, total: count(state) })).filter(item => item.total > 0))
const pending = computed(() => count('queued') + count('running'))
const processed = computed(() => rows.length - pending.value - count('cancelled'))
const progressLabel = computed(() => t('admin.providers.bulkProbe.progress', { done: processed.value, total: rows.length }))
const progressWidth = computed(() => rows.length ? `${processed.value / rows.length * 100}%` : '0%')

function stateLabel(state: ProbeState) {
  if (state === 'healthy' || state === 'degraded' || state === 'inconclusive') {
    return t(`pelicanTests.probe.status.${stateProbeStatus[state]}`)
  }
  return t(`admin.providers.bulkProbe.state.${state}`)
}

async function runQueue() {
  let index = 0
  const signal = controller.signal
  const runWorker = async () => {
    while (!signal.aborted && index < rows.length) {
      const row = rows[index++]
      row.state = 'running'
      try {
        const result = await runIQStateProbe(row.id, { model_id: stateProbeDefaultModel }, signal)
        if (signal.aborted) return
        row.result = result
        row.state = result.verdict
      } catch (error) {
        if (signal.aborted) return
        row.state = stateByErrorCode.get(extractApiErrorCode(error) ?? '') ?? 'failed'
      }
    }
  }
  await Promise.all(Array.from({ length: Math.min(PROBE_CONCURRENCY, rows.length) }, runWorker))
}

function stop() {
  // 同一个信号取消在途请求，并阻止迟到的响应覆盖停止后的结果。
  controller.abort()
  for (const row of rows) {
    if (row.state === 'queued' || row.state === 'running') row.state = 'cancelled'
  }
}

function close() {
  stop()
  emit('close')
}

onMounted(runQueue)
onBeforeUnmount(() => controller.abort())
</script>
