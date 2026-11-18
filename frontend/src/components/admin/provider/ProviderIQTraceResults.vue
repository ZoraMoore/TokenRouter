<template>
  <div class="space-y-3">
    <section v-if="result" class="space-y-3 rounded-surface border border-gray-200 p-4 dark:border-dark-600">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <h3 class="text-sm font-semibold">{{ t('providerIQ.traceConclusion') }}</h3>
        <span class="badge" :class="result.status === 'complete' ? 'badge-success' : 'badge-warning'">{{ t('providerIQ.traceStatus.' + result.status) }}</span>
      </div>
      <template v-if="result.analysis">
        <p class="break-words text-base font-semibold text-gray-900 dark:text-dark-50">{{ result.analysis.prediction }} · {{ percent(result.analysis.probability) }}</p>
        <p class="break-words text-xs text-gray-600 dark:text-dark-300">{{ result.requested_model }} · {{ result.reasoning_effort || t('pelicanTests.effort.default') }} · {{ t('providerIQ.traceUsed', { count: result.analysis.used_outputs }) }}</p>
        <ul v-if="result.analysis.candidates?.length" class="divide-y divide-gray-200 text-sm dark:divide-dark-600">
          <li v-for="candidate in result.analysis.candidates.slice(0, 5)" :key="candidate.model" class="flex justify-between gap-3 py-1.5">
            <span class="break-all">{{ candidate.model }}</span><span class="shrink-0 tabular-nums">{{ percent(candidate.probability) }}</span>
          </li>
        </ul>
        <details v-if="result.analysis.families?.length" class="text-sm">
          <summary class="cursor-pointer font-medium">{{ t('providerIQ.traceFamilies') }}</summary>
          <ul class="mt-2 space-y-1">
            <li v-for="family in result.analysis.families" :key="family.family" class="flex justify-between gap-3">
              <span class="break-all">{{ family.family }}</span><span class="shrink-0 tabular-nums">{{ percent(family.probability) }}</span>
            </li>
          </ul>
        </details>
      </template>
      <p v-if="result.disagreement" class="text-sm text-amber-700 dark:text-amber-300">{{ t('providerIQ.traceDisagreement') }}</p>
      <p v-if="result.bank.algorithm" class="break-words text-xs text-gray-500 dark:text-dark-400">{{ t('providerIQ.traceBank', { algorithm: result.bank.algorithm, count: result.bank.models.length }) }}</p>
    </section>
    <article v-for="(sample, i) in samples" :key="sample.challenge.id" class="space-y-2 rounded-surface border border-gray-200 p-3 dark:border-dark-600">
      <div class="flex flex-wrap items-center justify-between gap-2 text-sm">
        <h4 class="font-medium">{{ t('providerIQ.sample', { index: i + 1 }) }}</h4>
        <span class="badge" :class="sample.accepted ? 'badge-success' : 'badge-gray'">{{ t(sample.accepted ? 'providerIQ.traceAccepted' : 'providerIQ.traceExcluded') }}</span>
      </div>
      <p class="break-words text-xs text-gray-600 dark:text-dark-300">
        {{ t('providerIQ.traceNumbers', { observed: sample.parsed_numbers, minimum: sample.minimum_numbers }) }}
        <template v-if="sample.mapped_model"> · {{ sample.mapped_model }}</template>
        <template v-if="sample.environment?.protocol"> · {{ sample.environment.protocol }}</template>
        <template v-if="sample.latency_ms != null"> · {{ durationLabel(t, sample.latency_ms) }}</template>
      </p>
      <p v-if="sample.analysis" class="break-words text-sm">{{ sample.analysis.prediction }} · {{ percent(sample.analysis.probability) }}</p>
      <p v-if="sample.error" class="text-sm text-gray-600 dark:text-dark-300">{{ t(errorLabel(sample.error)) }}</p>
      <details v-if="sample.challenge.prompt || sample.text" class="text-sm">
        <summary class="cursor-pointer font-medium">{{ t('providerIQ.traceAnswer') }}</summary>
        <p class="mt-2 whitespace-pre-wrap break-words">{{ sample.challenge.prompt }}</p>
        <p v-if="sample.text" class="mt-2 max-h-52 overflow-y-auto whitespace-pre-wrap break-words rounded-control bg-gray-50 p-3 font-mono dark:bg-dark-800">{{ sample.text }}</p>
      </details>
    </article>
  </div>
</template>
<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { IQTraceResult, IQTraceSample } from '@/api/admin/providerIQ'
import { durationLabel } from '@/components/pelican/pelicanArtwork'

defineProps<{ result: IQTraceResult | null; samples: IQTraceSample[] }>()
const { t } = useI18n()
// 模型库返回 0 到 1 之间的占比，界面以百分比显示。
function percent(value: number) { return (value * 100).toFixed(1) + '%' }

// 指纹采样的错误代码对应本站提示，上游报错正文由服务端处理。
function errorLabel(code: string) {
  const labels: Record<string, string> = { too_few_numbers: 'traceTooFew', analysis_failed: 'traceAnalysisFailed', account_unavailable: 'unavailable', timeout: 'timeout', unsupported: 'unsupported' }
  if (['upstream_error', 'incomplete', 'empty_output', 'output_limit', 'unsupported_effort'].includes(code)) return 'pelicanTests.reason.' + code
  return 'providerIQ.' + (labels[code] || 'failed')
}
</script>
