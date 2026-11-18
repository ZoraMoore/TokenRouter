<template>
  <BaseDialog
    :show="resultId !== null"
    :title="t('pelicanTests.sample')"
    :subtitle="sample?.model_id"
    :z-index="review ? Z_INDEX.MODAL_NESTED : undefined"
    :width="!review && imageSource ? 'full' : 'wide'"
    @close="close"
  >
    <ContentSkeleton v-if="loading && !sample" variant="article" />
    <SettingsNotice v-if="failed" tone="error">
      {{ t('pelicanTests.sampleLoadFailed') }}
      <button type="button" class="ml-2 underline" @click="refresh">{{ t('common.retry') }}</button>
    </SettingsNotice>
    <div v-if="sample" class="space-y-4">
      <div class="flex flex-wrap items-center gap-2 text-sm text-gray-600 dark:text-dark-300">
        <PelicanStatus :status="sample.status" :probe="isProbe" />
        <span>{{ overriddenQuestionName(sample) || t(`pelicanTests.kind.${sample.question_kind}`) }}</span>
        <span v-if="!isProbe">{{ t('pelicanTests.reasoningEffort') }} {{ t(effortLabel(sample.reasoning_effort)) }}</span>
        <time :datetime="sample.tested_at">{{ formatDateTime(sample.tested_at) }}</time>
        <span class="tabular-nums">{{ t('pelicanTests.latency', { ms: sample.latency_ms }) }}</span>
        <span v-if="review && (sample.provider_name || sample.provider_id)">
          {{ t('pelicanTests.usedProvider', { name: sample.provider_name || '#' + sample.provider_id }) }}
        </span>
      </div>
      <SettingsNotice v-if="reason">{{ reason }}</SettingsNotice>

      <section v-if="sample.prompt || sample.criteria?.length" class="rounded-surface border border-gray-200 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-800">
        <h3 class="text-sm font-semibold text-gray-900 dark:text-dark-50">{{ t(isProbe ? 'pelicanTests.probe.method' : 'pelicanTests.prompt') }}</h3>
        <p class="mt-2 whitespace-pre-wrap break-words text-sm text-gray-700 dark:text-dark-200">{{ sample.prompt }}</p>
        <template v-if="sample.criteria?.length">
          <h4 class="mb-1 mt-4 text-sm font-semibold text-gray-900 dark:text-dark-50">{{ t(isProbe ? 'pelicanTests.probe.interpretation' : 'pelicanTests.criteria') }}</h4>
          <ul class="list-disc space-y-1 pl-5 text-sm text-gray-700 dark:text-dark-200">
            <li v-for="(criterion, index) in sample.criteria" :key="index">{{ criterion }}</li>
          </ul>
        </template>
      </section>
      <section>
        <div class="mb-2 flex flex-wrap items-center justify-between gap-2">
          <h3 class="text-sm font-semibold text-gray-900 dark:text-dark-50">
            {{ t(isProbe ? 'pelicanTests.probe.observation' : isPelicanArtwork(sample.question_kind) ? 'pelicanTests.artwork' : 'pelicanTests.answer') }}
          </h3>
          <div
            v-if="!review && imageSource && !imageFailed" v-segmented class="segmented"
            role="group" :aria-label="t('pelicanTests.showcase.previewSizing')"
          >
            <button
              v-for="mode in previewModes" :key="mode" type="button" class="segmented-item px-3 py-1.5 text-sm"
              :class="{ 'segmented-item-active': previewMode === mode }" :aria-pressed="previewMode === mode"
              :data-testid="`showcase-preview-${mode}`" @click="previewMode = mode"
            >{{ t(mode === 'fit' ? 'pelicanTests.showcase.fitArtwork' : 'pelicanTests.showcase.actualSize') }}</button>
          </div>
        </div>
        <PelicanArtworkPreview
          v-if="!review && imageSource && !imageFailed" :key="sample.id" :source="imageSource" :mode="previewMode"
          :alt="t('pelicanTests.artworkAlt', { model: sample.model_id })"
          class="h-[60dvh] rounded-surface border border-gray-200 dark:border-dark-700"
          @error="imageFailed = true"
        />
        <img
          v-else-if="imageSource && !imageFailed"
          :key="sample.id"
          :src="imageSource"
          :alt="t('pelicanTests.artworkAlt', { model: sample.model_id })"
          class="max-h-[60dvh] w-full rounded-surface border border-gray-200 bg-white object-contain dark:border-dark-700"
          @error="imageFailed = true"
        />
        <p
          v-else-if="!isPelicanArtwork(sample.question_kind) && sample.answer"
          class="whitespace-pre-wrap break-words rounded-surface bg-gray-50 p-4 font-mono text-sm dark:bg-dark-800"
        >{{ sample.answer }}</p>
        <p v-else class="text-sm text-gray-500 dark:text-dark-400">{{ t(isProbe ? 'pelicanTests.probe.noObservation' : 'pelicanTests.noAnswer') }}</p>
      </section>
      <SettingsNotice v-if="reviewFailed" tone="error">{{ t('pelicanTests.reviewFailed') }}</SettingsNotice>
    </div>

    <template #footer>
      <div class="flex flex-wrap justify-end gap-2">
        <button type="button" class="btn btn-secondary" :disabled="!!reviewing" @click="close">{{ t('common.close') }}</button>
        <template v-if="review && sample && isPelicanArtwork(sample.question_kind) && sample.status === 'pending_review'">
          <button
            type="button" class="btn btn-secondary" data-testid="review-suspected"
            :disabled="!!reviewing || loading" @click="submitReview('suspected')"
          >{{ t(reviewing === 'suspected' ? 'common.saving' : 'pelicanTests.markSuspected') }}</button>
          <button
            type="button" class="btn btn-primary" data-testid="review-passed"
            :disabled="!!reviewing || loading" @click="submitReview('passed')"
          >{{ t(reviewing === 'passed' ? 'common.saving' : 'pelicanTests.markPassed') }}</button>
        </template>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { AdminPelicanResultDetail } from '@/api/admin/pelicanTests'
import { isPelicanArtwork, type PelicanReviewStatus } from '@/api/pelicanTests'
import BaseDialog from '@/components/common/BaseDialog.vue'
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import { Z_INDEX } from '@/constants/overlay'
import { vSegmented } from '@/directives/segmented'
import { overriddenQuestionName } from '@/i18n/pelicanQuestionNames'
import { formatDateTime } from '@/utils/format'
import PelicanArtworkPreview from './PelicanArtworkPreview.vue'
import PelicanStatus from './PelicanStatus.vue'
import { effortLabel, pelicanImageSource } from './pelicanArtwork'
import { PELICAN_SLOW_POLL_MS, usePelicanResource } from './usePelicanResource'

const props = defineProps<{
  resultId: number | null
  loadResult: (id: number, signal: AbortSignal) => Promise<AdminPelicanResultDetail>
  review?: (id: number, status: PelicanReviewStatus) => Promise<void>
}>()
const emit = defineEmits<{ close: []; reviewed: [id: number, status: PelicanReviewStatus] }>()
const { t } = useI18n()
// 用户打开的作品可能已移入历史记录，按样本编号读取管理员给出的结论。
const { data: sample, loading, failed, refresh } = usePelicanResource(
  () => props.resultId,
  props.loadResult,
  result => !props.review && result.status === 'pending_review' ? PELICAN_SLOW_POLL_MS : undefined,
)
const reviewing = ref<PelicanReviewStatus | null>(null)
const reviewFailed = ref(false)
const imageFailed = ref(false)
const previewModes = ['fit', 'actual'] as const
const previewMode = ref<typeof previewModes[number]>('fit')
const isProbe = computed(() => sample.value?.question_kind === 'state_probe')

const probeReasons = new Set([
  'state_changed', 'account_error', 'rate_limited', 'model_unsupported', 'network_error', 'stream_error',
  'no_ticket', 'timeout', 'output_limit', 'invalid_state', 'incomplete', 'upstream_error', 'no_provider',
])
const knownReasons = new Set([
  'no_provider', 'upstream_error', 'incomplete', 'empty_output', 'output_limit', 'invalid_artwork', 'answer_mismatch', 'unsupported_effort',
])
const reason = computed(() => {
  const code = sample.value?.detail_code
  if (isProbe.value) return code && probeReasons.has(code) ? t(`pelicanTests.probe.reason.${code}`) : ''
  return code && knownReasons.has(code) ? t(`pelicanTests.reason.${code}`) : ''
})

const imageSource = computed(() => sample.value && isPelicanArtwork(sample.value.question_kind)
  ? pelicanImageSource(sample.value.answer || '') : '')

watch(() => props.resultId, () => {
  previewMode.value = 'fit'
  imageFailed.value = false
  reviewFailed.value = false
})

function close() {
  if (!reviewing.value) emit('close')
}

async function submitReview(status: PelicanReviewStatus) {
  const current = sample.value
  if (!props.review || !current || !isPelicanArtwork(current.question_kind) || current.status !== 'pending_review' || reviewing.value) return
  reviewing.value = status
  reviewFailed.value = false
  try {
    await props.review(current.id, status)
    if (props.resultId === current.id) sample.value = { ...current, status }
    emit('reviewed', current.id, status)
  } catch {
    reviewFailed.value = true
    refresh()
  } finally {
    reviewing.value = null
  }
}
</script>
