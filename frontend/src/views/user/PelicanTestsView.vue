<template>
  <AppLayout>
    <template #page-heading-actions>
      <div class="flex flex-wrap items-center gap-2">
        <button v-if="pelicanVisibility?.enabled" type="button" class="btn btn-secondary" data-testid="public-showcase-api-info" @click="apiOpen = true">
          <Icon name="terminal" size="sm" />{{ t('pelicanTests.showcase.apiTitle') }}
        </button>
        <button type="button" class="btn btn-secondary" data-testid="refresh-samples" :disabled="loading" @click="refreshSamples">
        <Icon name="refresh" size="sm" :class="{ 'animate-spin': loading }" />{{ t('common.refresh') }}
        </button>
      </div>
    </template>

    <div class="min-w-0 space-y-6">
      <SettingsNotice v-if="failed || accessLost" tone="error">{{ t('pelicanTests.samplesLoadFailed') }}</SettingsNotice>
      <div v-if="loading && !groups" class="card p-6"><ContentSkeleton /></div>
      <EmptyState v-else-if="groups && !groups.length" class="card" :title="t('pelicanTests.noGroups')" />
      <SettingsNotice v-if="drawings.length" role="note" class="text-sm leading-relaxed" data-testid="showcase-notice">
        {{ t('pelicanTests.showcase.artworkNotice') }}
      </SettingsNotice>
      <section v-for="question in questions" :key="question.key" class="min-w-0 space-y-3">
        <h2 class="flex flex-wrap items-center gap-2 break-words text-lg font-semibold text-gray-900 dark:text-dark-50">
          <Icon :name="question.drawing ? 'batchImage' : 'document'" size="md" class="shrink-0 text-primary-600 dark:text-primary-400" />
          <span class="min-w-0 break-all">{{ question.name || t(`pelicanTests.kind.${question.kind}`) }}</span>
          <span v-if="question.name && question.name !== t(`pelicanTests.kind.${question.kind}`)" class="text-xs font-normal text-gray-500 dark:text-dark-400">{{ t(`pelicanTests.kind.${question.kind}`) }}</span>
        </h2>
        <article
          v-for="row in question.rows" :key="row.group.group_id" class="overflow-hidden rounded-surface border border-gray-200 dark:border-dark-700"
          :class="question.drawing || row.samples.length ? 'bg-gradient-to-br from-emerald-50 via-slate-50 to-white dark:from-emerald-950/40 dark:via-dark-900 dark:to-dark-900' : 'bg-white dark:bg-dark-900'"
        >
          <header class="flex items-center gap-3 px-4 pt-4 sm:px-6 sm:pt-5">
            <span class="flex h-10 w-10 shrink-0 items-center justify-center rounded-surface bg-emerald-100/80 text-emerald-700 dark:bg-emerald-900/40 dark:text-emerald-300">
              <ModelIcon :model="row.models[0]?.model_id ?? row.samples[0]?.model_id ?? ''" size="20px" />
            </span>
            <div class="min-w-0">
              <h3 class="break-words text-base font-semibold text-gray-900 dark:text-dark-50">{{ row.group.group_name }}</h3>
              <p v-if="question.drawing || row.samples.length" class="mt-0.5 text-xs text-gray-600 dark:text-dark-300">
                {{ t(question.drawing ? 'pelicanTests.artworkCount' : 'pelicanTests.answerCount', { count: row.samples.length }) }}
                <template v-if="row.samples.length">
                  · {{ t('pelicanTests.latestArtwork', { time: formatRelativeTime(row.samples[0].tested_at) }) }}
                </template>
              </p>
            </div>
          </header>
          <ul
            v-if="row.samples.length" :data-testid="question.drawing ? 'artwork-strip' : 'answer-strip'"
            :aria-label="t(question.drawing ? 'pelicanTests.groupArtwork' : 'pelicanTests.groupAnswers', { group: row.group.group_name })"
            class="artwork-strip mt-4 flex snap-x snap-proximity gap-4 overflow-x-auto overscroll-x-contain px-4 pb-4 pt-1 scroll-px-4 sm:gap-5 sm:px-6 sm:scroll-px-6"
          >
            <PelicanSampleCard
              v-for="sample in row.samples" :key="sample.id" :sample="sample"
              :load-preview="loadPreview" @open="sampleId = $event"
            />
          </ul>
          <ul v-if="row.models.length" class="mx-4 divide-y divide-gray-200/75 py-2 text-xs text-gray-600 dark:divide-dark-700 dark:text-dark-300 sm:mx-6">
            <li
              v-for="model in row.models" :key="JSON.stringify([model.model_id, model.reasoning_effort || '', model.question_name || ''])"
              class="flex flex-wrap items-center gap-x-3 gap-y-2 py-2"
            >
              <span class="break-all font-mono text-sm text-gray-900 dark:text-dark-100">{{ model.model_id }}</span>
              <span v-if="question.kind !== 'state_probe'">{{ t('pelicanTests.configuredEffort') }} {{ t(effortLabel(model.reasoning_effort || 'default')) }}</span>
              <PelicanStatus v-if="model.latest" :status="model.latest.status" :probe="question.kind === 'state_probe'" />
              <span v-else class="badge badge-gray">{{ t('pelicanTests.noSample') }}</span>
              <span v-if="model.run_state && model.run_state !== 'idle'" class="badge badge-primary">{{ t(`pelicanTests.runState.${model.run_state}`) }}</span>
              <template v-if="model.latest && !row.samples.some(sample => sample.id === model.latest?.id)">
                <time :datetime="model.latest.tested_at">{{ formatDateTimeToMinute(model.latest.tested_at) }}</time>
                <span v-if="question.kind !== 'state_probe'">{{ t('pelicanTests.reasoningEffort') }} {{ t(effortLabel(model.latest.reasoning_effort)) }}</span>
                <span>{{ durationLabel(t, model.latest.latency_ms) }}</span>
                <button type="button" class="btn btn-secondary btn-sm" :data-testid="`open-sample-${model.latest.id}`" @click="sampleId = model.latest.id">
                  <Icon name="eye" size="sm" />{{ t('pelicanTests.openSample') }}
                </button>
              </template>
              <span v-if="!model.enabled" class="text-gray-500 dark:text-dark-400">{{ t('pelicanTests.paused') }}</span>
              <span v-else-if="model.next_run_at" class="text-gray-500 dark:text-dark-400">
                {{ t('pelicanTests.nextRun') }} <time :datetime="model.next_run_at">{{ formatDateTimeToMinute(model.next_run_at) }}</time>
              </span>
            </li>
          </ul>
        </article>
      </section>
      <section v-for="group in emptyGroups" :key="group.group_id" class="card p-4">
        <h2 class="text-base font-semibold">{{ group.group_name }}</h2>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ t('pelicanTests.noModels') }}</p>
      </section>
    </div>
    <PelicanShowcaseApiDialog v-if="apiOpen && pelicanVisibility?.enabled" :enabled="pelicanVisibility.api_enabled" :item-id="firstArtworkId" @close="apiOpen = false" />
    <PelicanResultDialog :result-id="sampleId" :load-result="loadResult" @close="sampleId = null" />
  </AppLayout>
</template>

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { getPelicanResult, isPelicanArtwork, listPelicanGroups } from '@/api/pelicanTests'
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import ModelIcon from '@/components/common/ModelIcon.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import Icon from '@/components/icons/Icon.vue'
import AppLayout from '@/components/layout/AppLayout.vue'
import PelicanSampleCard from '@/components/pelican/PelicanSampleCard.vue'
import PelicanResultDialog from '@/components/pelican/PelicanResultDialog.vue'
import PelicanStatus from '@/components/pelican/PelicanStatus.vue'
import PelicanShowcaseApiDialog from '@/components/pelican/PelicanShowcaseApiDialog.vue'
import { pelicanViewerId, pelicanVisibility, readPelicanVisibility } from '@/composables/usePelicanVisibility'
import { durationLabel, effortLabel, pelicanImageSource, pelicanQuestionSections } from '@/components/pelican/pelicanArtwork'
import { pelicanPollDelay, usePelicanResource } from '@/components/pelican/usePelicanResource'
import { formatDateTimeToMinute, formatRelativeTime } from '@/utils/format'

const { t, locale } = useI18n()
const sampleId = ref<number | null>(null)
const apiOpen = ref(false)
const accessLost = ref(false)
const previews = new Map<number, string>()
// 分组名由服务端按显示语言返回，切换语言时重新读取。
const { data: groups, loading, failed, refresh } = usePelicanResource(
  () => pelicanVisibility.value?.enabled && !accessLost.value ? JSON.stringify([pelicanViewerId.value, locale.value]) : null,
  (_key, signal) => readWithinGallery(signal, () => listPelicanGroups(signal)),
  items => pelicanPollDelay(items.flatMap(group => group.models)),
)
const questions = computed(() => pelicanQuestionSections(groups.value || []))
const emptyGroups = computed(() => groups.value?.filter(group => !group.models.length))

// 预览只在当前页面缓存。列表移除样本时一并释放图片，复核结论始终来自最新摘要和详情。
const drawings = computed(() => questions.value.filter(question => question.drawing)
  .flatMap(question => question.rows.flatMap(row => row.samples)))
const firstArtworkId = computed(() => drawings.value[0]?.id)
watch(drawings, samples => {
  const current = new Set(samples.map(sample => sample.id))
  for (const id of previews.keys()) {
    if (!current.has(id)) previews.delete(id)
  }
})

function clearGallery() {
  groups.value = null
  sampleId.value = null
  apiOpen.value = false
  previews.clear()
}

watch([pelicanViewerId, () => pelicanVisibility.value?.enabled], ([viewer, enabled], [previous]) => {
  if (!enabled || previous !== viewer) {
    clearGallery()
    accessLost.value = false
  }
}, { flush: 'sync' })

async function refreshSamples() {
  const viewer = pelicanViewerId.value
  if (viewer !== null && !(await readPelicanVisibility(viewer, true))?.enabled) {
    if (viewer === pelicanViewerId.value) {
      clearGallery()
      accessLost.value = true
    }
    return
  }
  if (viewer !== pelicanViewerId.value) return
  accessLost.value = false
  refresh()
}

// 用户失去分组权限、作品已移除或展示被关闭时，列表、打开的详情和缩略图同时失效。
async function readWithinGallery<T>(signal: AbortSignal, fetch: () => Promise<T>): Promise<T> {
  const viewer = pelicanViewerId.value
  try {
    const value = await fetch()
    if (signal.aborted || viewer !== pelicanViewerId.value || !pelicanVisibility.value?.enabled || accessLost.value) {
      throw new DOMException('展示内容已过期', 'AbortError')
    }
    return value
  } catch (error) {
    const status = (error as { status?: number } | null)?.status
    if (!signal.aborted && viewer === pelicanViewerId.value && (status === 401 || status === 403 || status === 404)) {
      clearGallery()
      accessLost.value = true
      if (viewer !== null) void readPelicanVisibility(viewer, true)
    }
    throw error
  }
}

function loadResult(id: number, signal: AbortSignal) {
  return readWithinGallery(signal, () => getPelicanResult(id, signal))
}

async function loadPreview(id: number, signal: AbortSignal): Promise<string> {
  const cached = previews.get(id)
  if (cached !== undefined) return cached
  const detail = await loadResult(id, signal)
  const source = isPelicanArtwork(detail.question_kind) ? pelicanImageSource(detail.answer || '') : ''
  if (!signal.aborted) previews.set(id, source)
  return source
}
</script>

<style scoped>
/* 保留可拖动的原生滚动条，鼠标和触屏都可向后翻阅。 */
.artwork-strip {
  scrollbar-width: thin;
  scrollbar-color: theme('colors.gray.400') transparent;
}
</style>
