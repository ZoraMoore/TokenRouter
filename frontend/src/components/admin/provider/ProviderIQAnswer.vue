<template>
  <article class="min-w-0 space-y-3 rounded-surface border border-gray-200 p-4 dark:border-dark-600">
    <header class="flex flex-wrap items-center justify-between gap-2">
      <h4 class="text-sm font-semibold text-gray-900 dark:text-dark-50">{{ t('providerIQ.sample', { index }) }}</h4>
      <div class="flex flex-wrap items-center gap-2">
        <PelicanStatus :status="result.status" />
        <button v-if="result.answer" type="button" class="btn btn-secondary btn-sm" data-testid="iq-download-answer" @click="downloadAnswer">
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
    <img v-if="source" :src="source" :alt="t('pelicanTests.artworkAlt', { model: result.requested_model })" class="max-h-[45dvh] w-full rounded-control border border-gray-200 bg-white object-contain dark:border-dark-600" />
    <p v-else-if="result.answer" class="max-h-64 overflow-y-auto whitespace-pre-wrap break-words rounded-control bg-gray-50 p-3 text-sm text-gray-900 dark:bg-dark-800 dark:text-dark-50">{{ result.answer }}</p>
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
import { durationLabel, pelicanImageSource } from '@/components/pelican/pelicanArtwork'
import PelicanStatus from '@/components/pelican/PelicanStatus.vue'
import { formatDateTimeToMinute } from '@/utils/format'

const props = defineProps<{ result: IQQuestionResult; index: number }>()
const { t } = useI18n()
const reasons = new Set(['no_provider', 'upstream_error', 'incomplete', 'empty_output', 'output_limit', 'invalid_artwork', 'answer_mismatch', 'unsupported_effort'])
const reason = computed(() => {
  const code = props.result.detail_code
  if (code === 'timeout' || code === 'unsupported') return 'providerIQ.' + code
  return code && reasons.has(code) ? 'pelicanTests.reason.' + code : ''
})
// 绘图响应走图片解码器，其他回答按纯文本显示。
const source = computed(() => ['pelican', 'custom_svg'].includes(props.result.question_kind) ? pelicanImageSource(props.result.answer || '') : '')
function effortText(value: string) {
  return value === 'default' || value === 'none' ? t('pelicanTests.effort.' + value) : value
}

// 文本文件保留回答原文，包括绘图模型写出的 SVG 标记。
function downloadAnswer() {
  if (!props.result.answer) return
  saveAs(new Blob([props.result.answer], { type: 'text/plain;charset=utf-8' }), `iq-answer-${Date.parse(props.result.started_at)}-${props.index}.txt`)
}
</script>
