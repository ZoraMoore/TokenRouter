<script setup lang="ts">
import { onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { getAPIActivity, type APIActivityStats } from '@/api/admin/users'

const { t } = useI18n()
const stats = ref<APIActivityStats | null>(null)
const unavailable = ref(false)
let controller: AbortController | null = null
let timer: ReturnType<typeof setInterval> | null = null
let disposed = false
let loading = false

// 统计独立于表格筛选和分页，页面卸载后停止轮询并取消未完成请求。
async function refresh() {
  if (disposed || loading) return
  loading = true
  controller = new AbortController()
  try {
    const data = await getAPIActivity({ signal: controller.signal })
    if (!Number.isInteger(data.active_users) || data.active_users < 0) throw new Error('Invalid activity count')
    if (!disposed) {
      stats.value = data
      unavailable.value = false
    }
  } catch {
    if (!disposed) unavailable.value = true
  } finally {
    loading = false
  }
}

function onVisibility() {
  if (document.visibilityState === 'visible') void refresh()
}

onMounted(() => {
  void refresh()
  document.addEventListener('visibilitychange', onVisibility)
  timer = setInterval(() => {
    if (document.visibilityState === 'visible') void refresh()
  }, 15000)
})

onUnmounted(() => {
  disposed = true
  document.removeEventListener('visibilitychange', onVisibility)
  if (timer) clearInterval(timer)
  controller?.abort()
})
</script>

<template>
  <div class="flex flex-wrap items-center gap-2 text-sm text-gray-600 dark:text-dark-300" role="status" aria-live="polite" :title="t('admin.users.activityDefinition')">
    <span>{{ t('admin.users.recentAPIUsers') }}</span>
    <span v-if="unavailable" class="text-amber-700 dark:text-amber-400">{{ t('admin.users.activityUnavailable') }}</span>
    <template v-else-if="stats">
      <span v-if="!stats.complete">{{ t('admin.users.activityApproximate') }}</span>
      <strong class="font-semibold tabular-nums text-emerald-700 dark:text-emerald-400" data-testid="api-active-user-count">{{ stats.active_users }}</strong>
      <span>{{ t('admin.users.activityPeople') }}</span>
      <span v-if="!stats.complete" class="text-xs text-gray-500 dark:text-dark-400">{{ t('admin.users.activitySyncing') }}</span>
    </template>
    <span v-else>{{ t('common.loading') }}</span>
  </div>
</template>
