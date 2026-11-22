<template>
  <div v-if="target > 0 || pending" class="space-y-1.5 py-1">
    <div class="flex flex-wrap items-baseline justify-between gap-x-3 gap-y-1 text-xs text-gray-600 dark:text-dark-300" aria-live="polite">
      <span class="font-medium tabular-nums text-gray-900 dark:text-gray-100">{{ progressText }}</span>
      <span v-if="target > 0" class="tabular-nums">
        {{ t('admin.providers.operations.progress.remaining', { count: target - reached }) }}
      </span>
    </div>
    <div
      role="progressbar"
      :aria-label="t('admin.providers.operations.progress.label')"
      :aria-valuemin="0"
      :aria-valuemax="target || undefined"
      :aria-valuenow="target > 0 ? reached : undefined"
      :aria-valuetext="progressText"
      class="h-2 overflow-hidden rounded-full bg-gray-200 dark:bg-dark-700"
    >
      <div
        v-if="target > 0"
        class="h-full rounded-full transition-[width] motion-reduce:transition-none"
        :class="fillClass"
        :style="{ width: `${reached / target * 100}%` }"
      />
      <div v-else class="h-full w-full animate-pulse rounded-full bg-primary-400/40 motion-reduce:animate-none" />
    </div>
    <p v-if="unconfirmed != null" class="text-xs tabular-nums text-gray-600 dark:text-dark-300">
      {{ t('admin.providers.operations.progress.unconfirmedSessions', { count: unconfirmed }) }}
    </p>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { ProviderOperation } from '@/api/admin/providers'
import { isProviderOperationPending } from '@/composables/useProviderOperationPolling'

const props = defineProps<{ operation: ProviderOperation }>()
const { t } = useI18n()
const pending = computed(() => isProviderOperationPending(props.operation))
const target = computed(() => Math.max(0, props.operation.result?.target_concurrency ?? 0))
const unconfirmed = computed(() => props.operation.result?.unconfirmed_sessions)

// 用任务观察到的峰值显示达标程度，实时并发下降时保留已经达成的进度。
const reached = computed(() => Math.min(target.value, Math.max(0, props.operation.result?.peak_concurrency ?? 0)))
const progressText = computed(() => target.value > 0
  ? t('admin.providers.operations.progress.concurrency', { reached: reached.value, target: target.value })
  : t('admin.providers.operations.progress.waiting'))

const fills: Record<ProviderOperation['state'], string> = {
  queued: 'bg-primary-500',
  running: 'bg-primary-500',
  succeeded: 'bg-emerald-500',
  failed: 'bg-red-500',
  needs_action: 'bg-amber-500'
}
const fillClass = computed(() => fills[props.operation.state])
</script>
