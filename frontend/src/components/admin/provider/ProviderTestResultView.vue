<template>
  <div class="flex min-h-0 flex-1 flex-col gap-4">
    <div v-if="$slots.leading" class="flex min-w-0 shrink-0 items-center gap-2">
      <slot name="leading"></slot>
    </div>

    <div class="grid shrink-0 grid-cols-3 divide-x divide-gray-200 rounded-surface border border-gray-200 dark:divide-dark-600 dark:border-dark-600">
      <div v-for="metric in metrics" :key="metric.key" class="min-w-0 px-4 py-3">
        <div class="truncate text-xs text-gray-500 dark:text-dark-400">{{ metric.label }}</div>
        <div
          class="mt-1.5 truncate font-mono text-base font-semibold tabular-nums text-gray-900 dark:text-dark-50"
          :title="metric.value"
        >
          {{ metric.value }}
        </div>
      </div>
    </div>

    <div class="flex min-h-64 flex-1 flex-col overflow-hidden rounded-surface border border-gray-200 dark:border-dark-600">
      <div class="flex h-11 shrink-0 items-center justify-between gap-2 border-b border-gray-200 px-4 dark:border-dark-600">
        <span class="truncate text-sm font-medium text-gray-700 dark:text-dark-200">
          {{ t('admin.providers.testDialog.output') }}
        </span>
        <div class="flex items-center gap-1">
          <button
            type="button"
            class="btn-icon-sm text-gray-500 hover:bg-gray-100 hover:text-gray-700 disabled:cursor-not-allowed disabled:opacity-40 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-dark-100"
            :disabled="!copyText"
            :title="t('admin.providers.testDialog.copy')"
            :aria-label="t('admin.providers.testDialog.copy')"
            @click="copyOutput"
          >
            <Icon name="copy" size="sm" />
          </button>
          <div v-segmented class="segmented" role="radiogroup" :aria-label="t('admin.providers.testDialog.output')">
            <button
              v-for="item in viewOptions"
              :key="item.value"
              type="button"
              role="radio"
              :aria-checked="outputView === item.value"
              :class="['segmented-item px-2.5 py-1 text-xs', { 'segmented-item-active': outputView === item.value }]"
              @click="outputView = item.value"
            >
              {{ item.label }}
            </button>
          </div>
        </div>
      </div>

      <div ref="outputRef" class="min-h-0 flex-1 overflow-auto overscroll-contain p-4" data-testid="provider-test-output">
        <template v-if="outputView === 'reply'">
          <SettingsNotice v-if="run.status === 'error'" tone="error" class="mb-3 break-words">
            {{ run.errorMessage }}
          </SettingsNotice>

          <div
            v-if="run.replyText"
            class="whitespace-pre-wrap break-words text-sm leading-relaxed text-gray-800 dark:text-dark-100"
          >{{ run.replyText }}<span v-if="running" class="ml-0.5 inline-block h-4 w-1.5 animate-pulse bg-primary-500 align-text-bottom"></span></div>

          <div v-if="run.images.length > 0" class="mt-3 flex flex-wrap gap-3">
            <button
              v-for="(image, index) in run.images"
              :key="`${image.url}-${index}`"
              type="button"
              class="group/img relative overflow-hidden rounded-surface border border-gray-200 bg-white transition hover:border-black/20 dark:border-dark-600 dark:bg-dark-900 dark:hover:border-dark-500"
              @click="previewImageUrl = image.url"
            >
              <img
                :src="image.url"
                :alt="t('admin.providers.imagePreviewAlt', { index: index + 1 })"
                class="max-h-64 w-full object-contain"
              />
              <span class="absolute inset-0 flex items-center justify-center bg-black/0 transition-colors group-hover/img:bg-black/20">
                <Icon
                  name="eye"
                  size="lg"
                  class="text-white opacity-0 drop-shadow-lg transition-opacity group-hover/img:opacity-100"
                />
              </span>
            </button>
          </div>

          <div
            v-if="!run.replyText && run.images.length === 0 && run.status !== 'error'"
            class="flex h-full min-h-40 flex-col items-center justify-center gap-2 px-4 text-center"
          >
            <Icon
              v-if="running"
              name="loader"
              size="xl"
              class="mb-2 animate-spin text-primary-500"
              :animate-on-hover="false"
            />
            <Icon v-else name="beaker" size="xl" class="mb-2 text-gray-400 dark:text-dark-500" />
            <div class="text-sm font-medium text-gray-900 dark:text-dark-50">{{ emptyTitle }}</div>
            <p class="max-w-sm text-xs leading-relaxed text-gray-500 dark:text-dark-400">{{ emptyDescription }}</p>
          </div>
        </template>

        <template v-else>
          <div v-if="run.logLines.length > 0" class="space-y-1 font-mono text-xs leading-relaxed">
            <div
              v-for="(line, index) in run.logLines"
              :key="index"
              :class="['break-words', LOG_TONE_CLASSES[line.tone]]"
            >
              {{ line.text }}
            </div>
          </div>
          <p v-else class="text-xs text-gray-500 dark:text-dark-400">
            {{ t('admin.providers.testDialog.logEmpty') }}
          </p>
        </template>
      </div>
    </div>

    <!-- 图片灯箱 -->
    <Teleport to="body">
      <MotionTransition name="fade">
        <div
          v-if="previewImageUrl"
          class="fixed inset-0 z-tooltip flex items-center justify-center bg-[var(--overlay-bg-strong)] p-4"
          @click.self="previewImageUrl = ''"
        >
          <button
            type="button"
            class="absolute right-4 top-4 rounded-full bg-black/50 p-2 text-white transition-colors hover:bg-black/70"
            :aria-label="t('common.close')"
            @click="previewImageUrl = ''"
          >
            <Icon name="x" size="lg" />
          </button>
          <img
            :src="previewImageUrl"
            :alt="t('admin.providers.imageLightboxAlt')"
            class="max-h-[90vh] max-w-[90vw] rounded-control object-contain shadow-2xl"
          />
        </div>
      </MotionTransition>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import MotionTransition from '@/components/common/MotionTransition.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import { Icon } from '@/components/icons'
import { vSegmented } from '@/directives/segmented'
import { useClipboard } from '@/composables/useClipboard'
import type { ProviderTestLogTone, ProviderTestRun } from './providerTestRun'

const props = defineProps<{
  run: ProviderTestRun
}>()

const { t } = useI18n()
const { copyToClipboard } = useClipboard()

const LOG_TONE_CLASSES: Record<ProviderTestLogTone, string> = {
  info: 'text-primary-600 dark:text-primary-400',
  muted: 'text-gray-500 dark:text-dark-400',
  success: 'text-green-600 dark:text-green-400',
  error: 'text-red-600 dark:text-red-400'
}

const outputRef = ref<HTMLElement | null>(null)
const outputView = ref<'reply' | 'log'>('reply')
const previewImageUrl = ref('')
const running = computed(() => props.run.status === 'connecting')

const viewOptions = computed(() => [
  { value: 'reply' as const, label: t('admin.providers.testDialog.viewReply') },
  { value: 'log' as const, label: t('admin.providers.testDialog.viewLog') }
])

const formatSeconds = (value: number | null) => (value == null ? '—' : `${(value / 1000).toFixed(2)} s`)

const metrics = computed(() => [
  { key: 'model', label: t('admin.providers.testDialog.metricModel'), value: props.run.resolvedModel || '—' },
  { key: 'first', label: t('admin.providers.testDialog.metricFirstToken'), value: formatSeconds(props.run.firstTokenMs) },
  { key: 'total', label: t('admin.providers.testDialog.metricTotal'), value: formatSeconds(props.run.totalMs) }
])

const emptyTitle = computed(() => {
  if (running.value) return t('admin.providers.testDialog.waitingTitle')
  if (props.run.status === 'success') return t('admin.providers.testDialog.noContentTitle')
  return t('admin.providers.testDialog.emptyTitle')
})
const emptyDescription = computed(() => {
  if (running.value) return t('admin.providers.testDialog.waitingDescription')
  if (props.run.status === 'success') return t('admin.providers.testDialog.noContentDescription')
  return t('admin.providers.testDialog.emptyDescription')
})

const copyText = computed(() =>
  outputView.value === 'log'
    ? props.run.logLines.map((line) => line.text).join('\n')
    : props.run.replyText
)

// 回复或日志增长时滚到底部，跟随流式输出。
watch(
  () => [props.run.replyText.length, props.run.logLines.length, outputView.value],
  async () => {
    await nextTick()
    if (outputRef.value) {
      outputRef.value.scrollTop = outputRef.value.scrollHeight
    }
  }
)

const copyOutput = () => {
  if (!copyText.value) return
  copyToClipboard(copyText.value, t('admin.providers.outputCopied'))
}
</script>
