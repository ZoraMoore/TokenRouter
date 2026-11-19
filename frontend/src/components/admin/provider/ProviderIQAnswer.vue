<template>
  <article class="min-w-0 space-y-3 rounded-surface border border-gray-200 p-4 dark:border-dark-600">
    <header class="flex flex-wrap items-center justify-between gap-2">
      <h4 class="text-sm font-semibold text-gray-900 dark:text-dark-50">{{ t('providerIQ.sample', { index }) }}</h4>
      <div class="flex flex-wrap items-center gap-2">
        <PelicanStatus :status="missingArtwork ? 'inconclusive' : result.status" />
        <button v-if="answerText" type="button" class="btn btn-secondary btn-sm" data-testid="iq-download-answer" @click="downloadAnswer">
          <Icon name="download" size="sm" />{{ t('providerIQ.downloadAnswer') }}
        </button>
      </div>
    </header>
    <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-gray-600 dark:text-dark-300">
      <span class="break-all font-mono">{{ result.requested_model }}</span>
      <span v-if="result.mapped_model && result.mapped_model !== result.requested_model" class="break-all">{{ t('providerIQ.actualModel', { model: result.mapped_model }) }}</span>
      <span v-if="result.protocol" class="break-all font-mono">{{ result.protocol }}</span>
      <span v-if="result.reasoning_effort">{{ t('providerIQ.effort') }} {{ effortText(result.reasoning_effort) }}</span>
      <span>{{ durationLabel(t, result.latency_ms) }}</span>
      <time :datetime="result.started_at">{{ formatDateTimeToMinute(result.started_at) }}</time>
    </div>
    <p v-if="reason" class="text-sm text-gray-600 dark:text-dark-300">{{ t(reason) }}</p>
    <PelicanArtworkPreview v-if="artwork" :artwork="artwork" interactive :alt="t('pelicanTests.artworkAlt', { model: result.requested_model })" class="h-[55dvh] rounded-control border border-gray-200 dark:border-dark-600" />
    <p v-else-if="!drawing && result.answer" class="max-h-64 overflow-y-auto whitespace-pre-wrap break-words rounded-control bg-gray-50 p-3 text-sm text-gray-900 dark:bg-dark-800 dark:text-dark-50">{{ result.answer }}</p>
    <details v-if="result.prompt || result.criteria?.length" class="text-sm text-gray-700 dark:text-dark-200">
      <summary class="cursor-pointer font-medium">{{ t('providerIQ.questionAndCriteria') }}</summary>
      <p class="mt-2 whitespace-pre-wrap break-words">{{ result.prompt }}</p>
      <ul v-if="result.criteria?.length" class="mt-2 list-disc space-y-1 pl-5">
        <li v-for="(criterion, i) in result.criteria" :key="i" class="break-words">{{ criterion }}</li>
      </ul>
    </details>
  </article>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { saveAs } from 'file-saver'
import type { IQQuestionResult } from '@/api/admin/providerIQ'
import Icon from '@/components/icons/Icon.vue'
import { isPelicanArtwork } from '@/api/pelicanTests'
import PelicanArtworkPreview from '@/components/pelican/PelicanArtworkPreview.vue'
import { durationLabel, pelicanArtworkBody } from '@/components/pelican/pelicanArtwork'
import PelicanStatus from '@/components/pelican/PelicanStatus.vue'
import { formatDateTimeToMinute } from '@/utils/format'

const props = defineProps<{ result: IQQuestionResult; index: number }>()
const { t } = useI18n()
const reasons = new Set(['no_provider', 'upstream_error', 'incomplete', 'empty_output', 'output_limit', 'invalid_artwork', 'answer_mismatch', 'unsupported_effort'])
const drawing = computed(() => isPelicanArtwork(props.result.question_kind))
const artwork = computed(() => pelicanArtworkBody(props.result))
const answerText = computed(() => props.result.raw_answer || props.result.answer)
const missingArtwork = computed(() => drawing.value && !!props.result.raw_answer && props.result.status !== 'inconclusive' && !artwork.value)
const reason = computed(() => {
  if (missingArtwork.value) return 'pelicanTests.reason.incomplete'
  const code = props.result.detail_code
  if (code === 'timeout' || code === 'unsupported') return 'providerIQ.' + code
  return code && reasons.has(code) ? 'pelicanTests.reason.' + code : ''
})
function effortText(value: string) {
  return value === 'default' || value === 'none' ? t('pelicanTests.effort.' + value) : value
}

// 绘图下载包含模型写在 SVG 外的脚本和样式。
function downloadAnswer() {
  if (!answerText.value) return
  saveAs(new Blob([answerText.value], { type: 'text/plain;charset=utf-8' }), `iq-answer-${Date.parse(props.result.started_at)}-${props.index}.txt`)
}
</script>
