<template>
  <BaseDialog :show="show" :title="t('providerIQ.title')" :subtitle="provider?.name" width="extra-wide" :body-scroll="false" flush @close="close">
    <template #header-icon>
      <span v-if="provider" class="flex h-10 w-10 items-center justify-center rounded-control border border-gray-200 bg-gray-50 dark:border-dark-600 dark:bg-dark-950">
        <PlatformIcon :platform="provider.platform" size="lg" :class="platformIconClass(provider.platform)" />
      </span>
    </template>
    <div v-if="provider" class="min-h-0 overflow-y-auto">
      <div class="border-b border-gray-200 px-4 py-3 dark:border-dark-600 sm:px-6">
        <SettingsSegmented :model-value="mode" :options="modes" :ariaLabel="t('providerIQ.modeLabel')" :disabled="busy" @update:model-value="setMode" />
      </div>
      <section class="flex min-h-0 flex-col md:h-[34rem] md:flex-row">
        <aside class="min-w-0 space-y-4 border-b border-gray-200 bg-gray-50/70 px-4 py-4 dark:border-dark-600 dark:bg-dark-950 sm:px-6 md:w-80 md:shrink-0 md:overflow-y-auto md:border-b-0 md:border-r">
          <div>
            <label for="provider-iq-model" class="input-label">{{ t('providerIQ.model') }}</label>
            <Select id="provider-iq-model" v-model="model" :options="models" :disabled="busy" searchable creatable data-testid="iq-model" :creatable-prefix="t('providerIQ.useModel')" :placeholder="mode === 'probe' ? stateProbeDefaultModel : undefined" />
            <p class="input-hint">{{ mode === 'probe' ? t('providerIQ.probe.modelHint', { model: stateProbeDefaultModel }) : t('providerIQ.modelHint') }}</p>
          </div>
          <div v-if="mode !== 'probe'" class="grid gap-3" :class="mode === 'question' ? 'grid-cols-2' : 'grid-cols-1'">
            <div>
              <label for="provider-iq-effort" class="input-label">{{ t('providerIQ.effort') }}</label>
              <Select id="provider-iq-effort" v-model="effort" :options="efforts" :disabled="busy || efforts.length === 1" data-testid="iq-effort" />
            </div>
            <div v-if="mode === 'question'">
              <label for="provider-iq-count" class="input-label">{{ t('providerIQ.count') }}</label>
              <Select id="provider-iq-count" v-model="count" :options="counts" :disabled="busy" data-testid="iq-count" />
            </div>
          </div>
          <template v-if="mode === 'question'">
            <div>
              <label for="provider-iq-kind" class="input-label">{{ t('pelicanTests.questionKind') }}</label>
              <Select id="provider-iq-kind" :model-value="kind" :options="kinds" :disabled="busy" data-testid="iq-kind" @update:model-value="setKind" />
            </div>
            <div>
              <label for="provider-iq-prompt" class="input-label">{{ t('pelicanTests.prompt') }}</label>
              <textarea id="provider-iq-prompt" v-model="prompt" rows="4" class="input" :disabled="busy" maxlength="8192" data-testid="iq-prompt" :placeholder="t(custom ? 'providerIQ.customPrompt' : 'providerIQ.defaultPrompt')" />
            </div>
            <div v-if="textQuestion">
              <label for="provider-iq-expected" class="input-label">{{ t('pelicanTests.expectedAnswer') }}</label>
              <input id="provider-iq-expected" v-model="expected" class="input" :disabled="busy" maxlength="1024" data-testid="iq-expected" :placeholder="t(kind === 'custom_text' ? 'providerIQ.optionalReference' : 'providerIQ.defaultReference')" />
            </div>
          </template>
          <p v-else class="text-sm leading-6 text-gray-600 dark:text-dark-300">{{ t(modeHints[mode]) }}</p>
          <SettingsNotice v-if="mode === 'probe' && probeIssue" data-testid="iq-probe-unsupported">{{ t(probeIssue) }}</SettingsNotice>
          <SettingsNotice v-if="probeScheduleModelTooLong">{{ t('providerIQ.probe.longScheduleModel') }}</SettingsNotice>
          <SettingsNotice v-if="mode === 'modeltrace' && traceIssue">{{ t(traceIssue) }}</SettingsNotice>
          <SettingsNotice v-if="validation" tone="error">{{ t(validation) }}</SettingsNotice>
        </aside>
        <section class="min-w-0 flex-1 space-y-4 px-4 py-4 sm:px-6 md:overflow-y-auto" :aria-label="t('providerIQ.results')">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <SettingsSegmented v-model="tab" :options="tabs" :ariaLabel="t('providerIQ.results')" :disabled="busy" />
            <button v-if="tab === 'results' && completed" type="button" class="btn btn-secondary btn-sm" data-testid="iq-download-round" @click="downloadRound">
              <Icon name="download" size="sm" />{{ t('providerIQ.downloadRound') }}
            </button>
          </div>
          <template v-if="tab === 'history'">
            <p class="input-hint">{{ t('providerIQ.historyHint') }}</p>
            <ul v-if="history.length" class="divide-y divide-gray-200 dark:divide-dark-600">
              <li v-for="item in history" :key="item.id">
                <button type="button" class="w-full space-y-1 rounded-control px-2 py-3 text-left hover:bg-gray-50 focus-visible:outline-primary-500 dark:hover:bg-dark-800" data-testid="iq-history-item" @click="showHistory(item)">
                  <span class="flex flex-wrap items-center justify-between gap-2 text-sm font-medium"><span>{{ t('providerIQ.modes.' + item.mode) }}</span><time class="text-xs text-gray-500 dark:text-dark-400" :datetime="item.at">{{ formatDateTimeToMinute(item.at) }}</time></span>
                  <span class="block break-all text-xs text-gray-600 dark:text-dark-300">
                    <span class="font-mono">{{ item.model }}</span> · {{ item.mode === 'probe' ? t('pelicanTests.probe.status.' + stateProbeStatus[item.result.verdict]) : item.effort || t('pelicanTests.effort.default') }}
                  </span>
                </button>
              </li>
            </ul>
            <p v-else class="rounded-surface border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">{{ t('providerIQ.emptyHistory') }}</p>
          </template>
          <template v-else>
            <SettingsNotice v-if="issue" tone="error">{{ t(issue) }}</SettingsNotice>
            <SettingsNotice v-if="saveFailed">{{ t('providerIQ.historyFailed') }}</SettingsNotice>
            <p v-if="cancelled" class="text-sm text-gray-600 dark:text-dark-300" role="status">{{ t('providerIQ.cancelled') }}</p>
            <p v-if="busy" class="flex items-center gap-2 text-sm text-primary-700 dark:text-primary-300" role="status">
              <Icon name="loader" size="sm" class="animate-spin" />{{ t(mode === 'probe' ? 'providerIQ.probe.running' : 'providerIQ.running') }}
              <span v-if="mode === 'modeltrace' && traceCount">{{ traceSamples.length }} / {{ traceCount }}</span>
            </p>
            <template v-if="mode === 'question' && slots.length">
              <div v-for="(slot, i) in slots" :key="i">
                <ProviderIQAnswer v-if="slot.result" :index="i + 1" :result="slot.result" />
                <p v-else class="flex items-center justify-between gap-2 rounded-control border border-gray-200 p-3 text-sm dark:border-dark-600"><span>{{ t('providerIQ.sample', { index: i + 1 }) }}</span><span>{{ t(slot.message || 'providerIQ.slot.' + slot.state) }}</span></p>
              </div>
            </template>
            <ProviderIQProbeResult v-else-if="mode === 'probe' && probe" :result="probe" />
            <template v-else-if="mode === 'quality' && quality">
              <section class="rounded-surface border border-gray-200 p-4 dark:border-dark-600">
                <span class="badge" :class="quality.verdict === 'passed' ? 'badge-success' : quality.verdict === 'mismatch' ? 'badge-danger' : 'badge-gray'">{{ t('providerIQ.qualityVerdict.' + quality.verdict) }}</span>
                <p class="mt-2 text-sm">{{ t('providerIQ.qualityScore', { passed: quality.passed, total: quality.total }) }} · {{ durationLabel(t, quality.latency_ms) }}</p>
              </section>
              <ProviderIQAnswer v-for="(sample, i) in quality.samples" :key="i" :index="i + 1" :result="sample" />
            </template>
            <ProviderIQTraceResults v-else-if="mode === 'modeltrace' && (trace || traceSamples.length)" :result="trace" :samples="traceSamples" />
            <p v-else-if="!busy && !cancelled && !issue" class="rounded-surface border border-dashed border-gray-300 p-6 text-center text-sm text-gray-500 dark:border-dark-600 dark:text-dark-400">{{ t(mode === 'probe' ? 'providerIQ.probe.empty' : 'providerIQ.noResults') }}</p>
          </template>
        </section>
      </section>
    </div>
    <template #footer>
      <div class="flex flex-wrap items-center justify-between gap-2">
        <RouterLink v-if="canSchedule" :to="scheduleTo" class="btn btn-secondary" data-testid="iq-schedule" @click="close"><Icon name="calendar" size="sm" />{{ t('providerIQ.schedule') }}</RouterLink>
        <div class="ml-auto flex flex-wrap gap-2">
          <button type="button" class="btn btn-secondary" @click="close">{{ t('common.close') }}</button>
          <button v-if="busy" type="button" class="btn btn-secondary" data-testid="iq-cancel" @click="cancel">{{ t('providerIQ.cancel') }}</button>
          <button v-else type="button" class="btn btn-primary" data-testid="iq-run" :disabled="!canRun" @click="run"><Icon name="play" size="sm" />{{ t(mode === 'probe' ? 'providerIQ.probe.start' : 'providerIQ.start') }}</button>
        </div>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { Provider } from '@/types'
import { stateProbeDefaultModel, stateProbeStatus } from '@/api/admin/providerIQ'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select from '@/components/common/Select.vue'
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import SettingsSegmented from '@/components/common/settings/SettingsSegmented.vue'
import Icon from '@/components/icons/Icon.vue'
import { durationLabel } from '@/components/pelican/pelicanArtwork'
import { platformIconClass } from '@/utils/platformColors'
import { formatDateTimeToMinute } from '@/utils/format'
import ProviderIQAnswer from './ProviderIQAnswer.vue'
import ProviderIQProbeResult from './ProviderIQProbeResult.vue'
import ProviderIQTraceResults from './ProviderIQTraceResults.vue'
import { useProviderIQTest } from './useProviderIQTest'

const props = defineProps<{ show: boolean; provider: Provider | null }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const modeHints = { probe: 'providerIQ.probe.hint', quality: 'providerIQ.qualityHint', modeltrace: 'providerIQ.traceHint' }
const {
  mode, modes, setMode, tab, tabs, model, models, effort, efforts, count, counts, kind, kinds, setKind, custom, textQuestion,
  prompt, expected, validation, scheduleTo, canSchedule, probeIssue, probeScheduleModelTooLong, traceIssue, slots, history, showHistory, quality, probe, trace, traceSamples,
  traceCount, busy, cancelled, issue, saveFailed, canRun, run, cancel, completed, downloadRound,
} = useProviderIQTest(props)
// 关闭窗口也会终止当前提供商的上游请求。
function close() { cancel(); emit('close') }
</script>
