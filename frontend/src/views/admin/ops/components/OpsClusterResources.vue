<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { OpsClusterResources } from '@/api/admin/ops'

defineProps<{ cluster: OpsClusterResources }>()
const { t } = useI18n()

// 资源接口的容量字段使用 MiB，展示单位与原始刻度保持一致。
function capacity(value?: number | null): string {
  if (value == null || !Number.isFinite(value) || value < 0) return '-'
  if (value < 1024) return `${Math.round(value)} MiB`
  return `${Number((value / 1024).toFixed(1))} GiB`
}

function percent(value?: number | null): string {
  return value == null || !Number.isFinite(value) ? '-' : `${value.toFixed(1)}%`
}

function cores(value?: number | null): string {
  return value != null && Number.isFinite(value) && value > 0 ? String(Number(value.toFixed(1))) : '-'
}

function sampledAt(value: string): string {
  const date = new Date(value)
  return Number.isNaN(date.getTime()) || date.getUTCFullYear() < 2000 ? '-' : date.toLocaleTimeString()
}

function usageClass(value?: number | null): string {
  if (value == null) return 'text-gray-900 dark:text-white'
  if (value >= 95) return 'text-rose-600 dark:text-rose-400'
  if (value >= 80) return 'text-amber-700 dark:text-amber-400'
  return 'text-emerald-700 dark:text-emerald-400'
}
</script>

<template>
  <section class="mb-4" :aria-label="t('admin.ops.clusterResources')" data-testid="ops-cluster-resources">
    <div class="mb-3 flex flex-wrap items-center justify-between gap-2">
      <h3 class="text-sm font-semibold text-gray-900 dark:text-white">{{ t('admin.ops.clusterResources') }}</h3>
      <span class="text-xs text-gray-600 dark:text-gray-300">
        {{ t('admin.ops.onlineNodes') }} {{ cluster.online_nodes }} / {{ cluster.total_nodes }}
      </span>
    </div>

    <p v-if="!cluster.complete" class="mb-3 text-xs text-amber-700 dark:text-amber-400" role="status">
      {{ t('admin.ops.clusterPartial') }}
    </p>

    <div class="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-3">
      <div class="rounded-surface bg-gray-50 p-3 dark:bg-dark-950">
        <div class="text-xs font-semibold text-gray-600 dark:text-gray-300">{{ t('admin.ops.hostCPU') }}</div>
        <div class="mt-1 text-lg font-bold tabular-nums" :class="usageClass(cluster.summary.host_cpu_usage_percent)">
          {{ percent(cluster.summary.host_cpu_usage_percent) }}
        </div>
        <div class="mt-1 text-xs text-gray-600 dark:text-gray-300">
          {{ t('admin.ops.cpuCores', { count: cores(cluster.summary.host_cpu_cores) }) }}
          · {{ t('admin.ops.appCPUCapacity', { count: cores(cluster.summary.cpu_cores) }) }}
        </div>
      </div>
      <div class="rounded-surface bg-gray-50 p-3 dark:bg-dark-950">
        <div class="text-xs font-semibold text-gray-600 dark:text-gray-300">{{ t('admin.ops.hostMemory') }}</div>
        <div class="mt-1 text-lg font-bold tabular-nums text-gray-900 dark:text-white" data-testid="cluster-host-memory">
          {{ capacity(cluster.summary.host_memory_used_mb) }} / {{ capacity(cluster.summary.host_memory_total_mb) }}
        </div>
        <div class="mt-1 text-xs text-gray-600 dark:text-gray-300" data-testid="cluster-app-memory">
          {{ t('admin.ops.appMemory') }} {{ capacity(cluster.summary.memory_used_mb) }} / {{ capacity(cluster.summary.memory_total_mb) }}
        </div>
      </div>
      <div class="rounded-surface bg-gray-50 p-3 dark:bg-dark-950">
        <div class="text-xs font-semibold text-gray-600 dark:text-gray-300">{{ t('admin.ops.diskTotal') }}</div>
        <div class="mt-1 text-lg font-bold tabular-nums text-gray-900 dark:text-white">{{ capacity(cluster.summary.disks?.total_mb) }}</div>
        <div class="mt-1 text-xs text-gray-600 dark:text-gray-300">
          {{ t('admin.ops.diskUsedFree', { used: capacity(cluster.summary.disks?.used_mb), free: capacity(cluster.summary.disks?.available_mb) }) }}
        </div>
      </div>
    </div>

    <div class="overflow-x-auto rounded-surface border border-gray-200 dark:border-dark-700">
      <table class="w-full text-left text-xs">
        <caption class="sr-only">{{ t('admin.ops.clusterNodeDetails') }}</caption>
        <thead class="bg-gray-50 text-gray-600 dark:bg-dark-950 dark:text-gray-300">
          <tr>
            <th scope="col" class="px-3 py-2 font-semibold">{{ t('admin.ops.serverNode') }}</th>
            <th scope="col" class="px-3 py-2 font-semibold">CPU</th>
            <th scope="col" class="px-3 py-2 font-semibold">{{ t('admin.ops.hostMemory') }}</th>
            <th scope="col" class="px-3 py-2 font-semibold">{{ t('admin.ops.appMemory') }}</th>
            <th scope="col" class="px-3 py-2 font-semibold">{{ t('admin.ops.disk') }}</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-gray-100 dark:divide-dark-700">
          <tr v-for="node in cluster.nodes" :key="node.node_id" :data-node-id="node.node_id">
            <th scope="row" class="px-3 py-3 align-top font-normal">
              <div class="whitespace-nowrap font-semibold text-gray-900 dark:text-white">{{ node.name }}</div>
              <div class="mt-1" :class="node.online ? 'text-emerald-700 dark:text-emerald-400' : 'text-rose-600 dark:text-rose-400'">
                {{ t(node.online ? 'admin.ops.nodeOnline' : 'admin.ops.nodeOffline') }}
              </div>
              <div class="mt-1 whitespace-nowrap text-gray-500 dark:text-gray-400">
                {{ t('admin.ops.nodeLastSample') }} {{ sampledAt(node.sampled_at) }}
              </div>
            </th>
            <td class="px-3 py-3 align-top tabular-nums">
              <div :class="usageClass(node.online ? node.host_cpu_usage_percent : null)">{{ node.online ? percent(node.host_cpu_usage_percent) : '-' }}</div>
              <div class="mt-1 whitespace-nowrap text-gray-600 dark:text-gray-300">{{ t('admin.ops.cpuCores', { count: cores(node.host_cpu_cores) }) }}</div>
              <div class="mt-1 whitespace-nowrap text-gray-600 dark:text-gray-300">{{ t('admin.ops.appCPUCapacity', { count: cores(node.cpu_cores) }) }}</div>
            </td>
            <td class="whitespace-nowrap px-3 py-3 align-top tabular-nums text-gray-900 dark:text-white">
              {{ node.online ? capacity(node.host_memory_used_mb) : '-' }} / {{ capacity(node.host_memory_total_mb) }}
            </td>
            <td class="whitespace-nowrap px-3 py-3 align-top tabular-nums text-gray-900 dark:text-white">
              {{ node.online ? capacity(node.memory_used_mb) : '-' }} / {{ capacity(node.memory_total_mb) }}
            </td>
            <td class="px-3 py-3 align-top tabular-nums text-gray-900 dark:text-white">
              <div class="whitespace-nowrap">{{ node.online ? capacity(node.disks?.used_mb) : '-' }} / {{ capacity(node.disks?.total_mb) }}</div>
              <div v-for="disk in node.disks?.disks ?? []" :key="disk.mount_point" class="mt-1 whitespace-nowrap text-gray-600 dark:text-gray-300">
                {{ disk.mount_point }} {{ node.online ? capacity(disk.used_mb) : '-' }} / {{ capacity(disk.total_mb) }}
              </div>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <p class="mt-2 text-xs text-gray-600 dark:text-gray-300">{{ t('admin.ops.clusterCapacityNote') }}</p>
    <p class="mt-1 text-xs text-gray-600 dark:text-gray-300 sm:hidden">{{ t('admin.ops.clusterSwipe') }}</p>
  </section>
</template>
