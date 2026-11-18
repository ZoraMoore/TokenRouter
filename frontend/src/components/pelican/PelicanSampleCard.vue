<template>
  <li ref="card" class="w-[min(78vw,17rem)] shrink-0 snap-start sm:w-72">
    <button
      type="button" :data-testid="`open-sample-${sample.id}`"
      class="group block w-full overflow-hidden rounded-surface border border-gray-200 bg-white text-left shadow-sm transition-shadow hover:shadow-md focus-visible:outline focus-visible:outline-2 focus-visible:outline-primary-500 dark:border-dark-700 dark:bg-dark-800"
      :aria-label="drawing ? t('pelicanTests.openArtwork', { model: sample.model_id, time: formatDateTimeToMinute(sample.tested_at) }) : undefined"
      @focus="requestPreview" @pointerenter="!observer && requestPreview()" @click="emit('open', sample.id)"
    >
      <span v-if="drawing" class="relative flex aspect-[4/3] w-full items-center justify-center overflow-hidden border-b border-gray-100 bg-gray-50 dark:border-dark-700 dark:bg-dark-900">
        <PelicanArtworkPreview
          v-if="source && !imageFailed" class="absolute inset-0 h-full"
          :source="source" :alt="t('pelicanTests.artworkAlt', { model: sample.model_id })" @error="imageFailed = true"
        />
        <span v-else-if="failed || imageFailed || source === ''" class="p-4 text-center text-xs text-gray-500 dark:text-dark-400">
          {{ t('pelicanTests.previewUnavailable') }}
        </span>
        <Icon v-else :name="loading ? 'loader' : 'modalityImage'" size="lg" class="text-gray-400 dark:text-dark-500" :class="{ 'animate-spin': loading }" />
        <span class="absolute right-2 top-2 opacity-0 transition-opacity group-hover:opacity-100 group-focus-visible:opacity-100">
          <PelicanStatus :status="sample.status" />
        </span>
      </span>
      <span v-else class="flex h-40 flex-col gap-2 border-b border-gray-100 bg-gray-50 p-4 dark:border-dark-700 dark:bg-dark-900">
        <span class="sr-only">{{ t('pelicanTests.openSample') }}</span>
        <PelicanStatus class="self-start" :status="sample.status" />
        <span class="flex min-h-0 flex-1 flex-col justify-center">
          <span
            class="line-clamp-4 whitespace-pre-wrap break-words text-gray-900 dark:text-dark-50"
            :class="shortAnswer ? 'text-3xl font-semibold leading-tight tabular-nums sm:text-4xl' : 'text-base leading-6'"
          >{{ answerPreview }}</span>
        </span>
      </span>
      <span class="block space-y-1 px-4 py-3">
        <span class="flex items-center justify-between gap-2">
          <span class="truncate font-mono text-sm font-medium text-gray-900 dark:text-dark-50" :title="sample.model_id">{{ sample.model_id }}</span>
          <span class="shrink-0 rounded-compact bg-gray-100 px-2 py-0.5 text-xs font-medium text-gray-600 dark:bg-dark-700 dark:text-dark-200">
            {{ t('pelicanTests.reasoningEffort') }} {{ t(effortLabel(sample.reasoning_effort)) }}
          </span>
        </span>
        <span class="flex items-baseline justify-between gap-2 text-xs tabular-nums text-gray-500 dark:text-dark-400">
          <time :datetime="sample.tested_at" :title="formatDateTime(sample.tested_at)">{{ formatDateTimeToMinute(sample.tested_at) }}</time>
          <span>{{ durationLabel(t, sample.latency_ms) }}</span>
        </span>
      </span>
    </button>
  </li>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { isPelicanArtwork, type PelicanCandyAnswer, type PelicanResult } from '@/api/pelicanTests'
import Icon from '@/components/icons/Icon.vue'
import { formatDateTime, formatDateTimeToMinute } from '@/utils/format'
import { durationLabel, effortLabel } from './pelicanArtwork'
import PelicanArtworkPreview from './PelicanArtworkPreview.vue'
import PelicanStatus from './PelicanStatus.vue'
import { usePelicanResource } from './usePelicanResource'

const props = defineProps<{ sample: PelicanResult | PelicanCandyAnswer; loadPreview: (id: number, signal: AbortSignal) => Promise<string> }>()
const emit = defineEmits<{ open: [id: number] }>()
const { t } = useI18n()
const drawing = computed(() => isPelicanArtwork(props.sample.question_kind))
const answerPreview = computed(() => 'answer_preview' in props.sample ? props.sample.answer_preview : '')
// 单行短答案放大显示，较长的推导按正文换行。
const shortAnswer = computed(() => answerPreview.value.length <= 12 && !/[\r\n]/.test(answerPreview.value))
const card = ref<HTMLElement | null>(null)
const visible = ref(false)
const imageFailed = ref(false)
let observer: IntersectionObserver | null = null
const { data: source, failed, loading, refresh } = usePelicanResource(() => visible.value && drawing.value ? props.sample.id : null, props.loadPreview)

function requestPreview() {
  if (!drawing.value) return
  if (failed.value) refresh()
  visible.value = true
}

// 浏览器同时检查页面视口和横向滚动容器，仅在作品可见时读取正文。键盘聚焦也能加载预览。
onMounted(() => {
  if (!drawing.value || !card.value || typeof IntersectionObserver === 'undefined') return
  observer = new IntersectionObserver(entries => {
    if (entries.some(entry => entry.isIntersecting)) {
      requestPreview()
      observer?.disconnect()
    }
  })
  observer.observe(card.value)
})
onBeforeUnmount(() => observer?.disconnect())
</script>
