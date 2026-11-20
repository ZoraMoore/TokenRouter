<template>
  <ul
    v-if="operations.length"
    class="divide-y divide-gray-100 dark:divide-dark-600"
    :class="flat ? undefined : 'overflow-hidden rounded-control border border-gray-200 dark:border-dark-600'"
    :aria-label="t('admin.providers.operations.history')"
  >
    <li v-for="operation in operations" :key="operation.id" class="space-y-1 px-3 py-2 text-sm">
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex min-w-0 flex-wrap items-baseline gap-x-2">
          <span class="break-all font-medium text-gray-900 dark:text-gray-100">{{ operation.provider_name || `#${operation.provider_id}` }}</span>
          <span class="text-xs text-gray-500 dark:text-dark-400">{{ t(`admin.providers.operations.kinds.${operation.kind}`) }}</span>
        </div>
        <span class="rounded-compact px-2 py-1 text-xs font-medium" :class="stateClasses[displayState(operation)]">
          {{ t(`admin.providers.operations.states.${displayState(operation)}`) }}
        </span>
      </div>
      <p v-if="isProviderOperationPending(operation) && operation.stage && te(`admin.providers.operations.stages.${operation.stage}`)" class="text-xs text-gray-600 dark:text-dark-300">
        {{ t(`admin.providers.operations.stages.${operation.stage}`) }}
      </p>
      <ProviderTrafficPushProgress v-if="operation.kind === 'traffic_push'" :operation="operation" />
      <p v-if="operation.message" class="break-words text-xs text-gray-700 dark:text-dark-200">{{ operation.message }}</p>
      <dl v-if="operation.result" class="flex flex-wrap gap-x-4 gap-y-1 text-xs text-gray-600 dark:text-dark-300">
        <div v-if="operation.kind === 'relogin' && operation.result.provider_id != null" class="flex gap-1">
          <dt>{{ t('admin.providers.operations.result.providerId') }}</dt>
          <dd class="tabular-nums">#{{ operation.result.provider_id }}</dd>
        </div>
        <template v-if="operation.kind === 'traffic_push' && (operation.stage || operation.state !== 'queued')">
          <div v-if="operation.result.migrated_sessions != null" class="flex gap-1">
            <dt>{{ t('admin.providers.operations.result.migratedSessions') }}</dt>
            <dd class="tabular-nums">{{ operation.result.migrated_sessions }}</dd>
          </div>
          <div v-if="operation.result.successful_requests != null" class="flex gap-1">
            <dt>{{ t('admin.providers.operations.result.successfulRequests') }}</dt>
            <dd class="tabular-nums">{{ operation.result.successful_requests }}</dd>
          </div>
          <div v-if="operation.result.peak_concurrency != null" class="flex gap-1">
            <dt>{{ t('admin.providers.operations.result.peakConcurrency') }}</dt>
            <dd class="tabular-nums">{{ operation.result.peak_concurrency }}</dd>
          </div>
        </template>
      </dl>
    </li>
  </ul>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { ProviderOperation } from '@/api/admin/providers'
import { isProviderOperationPending } from '@/composables/useProviderOperationPolling'
import ProviderTrafficPushProgress from './ProviderTrafficPushProgress.vue'

defineProps<{ operations: ProviderOperation[]; flat?: boolean }>()

const { t, te } = useI18n()

// 任务状态同时使用文字与颜色，执行中的状态使用品牌色。
const stateClasses: Record<ProviderOperation['state'], string> = {
  queued: 'bg-gray-100 text-gray-700 dark:bg-dark-700 dark:text-dark-200',
  running: 'bg-primary-50 text-primary-700 dark:bg-primary-900/30 dark:text-primary-300',
  succeeded: 'bg-emerald-50 text-emerald-800 dark:bg-emerald-900/30 dark:text-emerald-200',
  failed: 'bg-red-50 text-red-700 dark:bg-red-900/30 dark:text-red-300',
  needs_action: 'bg-amber-50 text-amber-800 dark:bg-amber-900/30 dark:text-amber-200'
}

// 已开始的推流会回到队列等观测结果，操作记录显示为执行中。
function displayState(operation: ProviderOperation): ProviderOperation['state'] {
  return operation.kind === 'traffic_push' && operation.state === 'queued' && operation.stage
    ? 'running'
    : operation.state
}
</script>
