<template>
  <div
    class="flex flex-wrap items-center gap-x-4 gap-y-2 text-xs text-gray-500 dark:text-dark-400"
    data-testid="live-stats"
    :aria-busy="!loaded"
  >
    <span
      v-for="item in items"
      :key="item.key"
      class="inline-flex items-center gap-2"
      :title="item.hint"
      :data-testid="`live-stat-${item.key}`"
    >
      <Icon :name="item.icon" size="xs" :class="METRIC_TONES[item.tone].icon" :animate-on-hover="false" />
      <span>{{ item.label }}</span>
      <Skeleton v-if="!loaded" :width="40" :height="12" />
      <span v-else class="font-medium tabular-nums text-gray-900 dark:text-dark-100">
        <span aria-hidden="true">{{ item.animated }}</span>
        <span class="sr-only">{{ item.value }}</span>
      </span>
    </span>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import Skeleton from '@/components/common/Skeleton.vue'
import Icon from '@/components/icons/Icon.vue'
import type { IconName } from '@/components/icons/registry'
import { usageAPI, type UserDashboardStats } from '@/api/usage'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import { useCountUp } from '@/composables/useCountUp'
import { formatNumberLocaleString as formatNumber, formatTokensK } from '@/utils/format'
import { COUNT_UP_MS } from './dashboardMotion'
import { METRIC_TONES, type MetricTone } from './metricTones'

// 页面可见时的轮询间隔；接口统计的是近 5 分钟均值，一分钟刷新一次足够。
const POLL_INTERVAL_MS = 60 * 1000

const { t } = useI18n()
const { formatBalanceAmount } = useBalanceDisplay()

const stats = ref<UserDashboardStats | null>(null)
const loaded = ref(false)
let timer: ReturnType<typeof setInterval> | null = null
// 用递增序号丢弃过期响应，手动刷新和轮询同时进行时只保留最后一次结果。
let requestSeq = 0

const animatedRpm = useCountUp(() => stats.value?.rpm ?? 0, COUNT_UP_MS)
const animatedTpm = useCountUp(() => stats.value?.tpm ?? 0, COUNT_UP_MS)
const animatedLatency = useCountUp(() => stats.value?.average_duration_ms ?? 0, COUNT_UP_MS)
const animatedTodayCost = useCountUp(() => stats.value?.today_actual_cost ?? 0, COUNT_UP_MS)

// formatLatency 不足 1 秒显示毫秒，否则显示秒。
const formatLatency = (ms: number): string => (ms < 1000 ? `${Math.round(ms)}ms` : `${(ms / 1000).toFixed(2)}s`)

// formatCost 将小额消费显示为 4 位小数。
const formatCost = (value: number): string => formatBalanceAmount(value, { fractionDigits: value >= 1 ? 2 : 4 })

// 取数失败时数值显示破折号，不弹出错误。
const display = (format: (value: number) => string, value: number | undefined): string => (
  value === undefined ? '—' : format(value)
)

const items = computed(() => {
  const current = stats.value
  return [
    {
      key: 'rpm',
      icon: 'swap' as IconName,
      tone: 'requests' as MetricTone,
      label: t('dashboard.live.rpm'),
      hint: t('dashboard.live.rpmHint'),
      value: display((value) => formatNumber(Math.round(value * 10) / 10), current?.rpm),
      animated: display((value) => formatNumber(Math.round(value * 10) / 10), current ? animatedRpm.value : undefined),
    },
    {
      key: 'tpm',
      icon: 'layers' as IconName,
      tone: 'tokens' as MetricTone,
      label: t('dashboard.live.tpm'),
      hint: t('dashboard.live.tpmHint'),
      value: display(formatTokensK, current?.tpm),
      animated: display(formatTokensK, current ? animatedTpm.value : undefined),
    },
    {
      key: 'latency',
      icon: 'clock' as IconName,
      tone: 'latency' as MetricTone,
      label: t('dashboard.live.latency'),
      hint: undefined,
      value: display(formatLatency, current?.average_duration_ms),
      animated: display(formatLatency, current ? animatedLatency.value : undefined),
    },
    {
      key: 'todayCost',
      icon: 'dollar' as IconName,
      tone: 'cost' as MetricTone,
      label: t('dashboard.live.todayCost'),
      hint: undefined,
      value: display(formatCost, current?.today_actual_cost),
      animated: display(formatCost, current ? animatedTodayCost.value : undefined),
    },
  ]
})

// load 读取实时统计，失败时清空数据但保留状态条。
const load = async () => {
  const seq = ++requestSeq
  try {
    const result = await usageAPI.getDashboardStats()
    if (seq !== requestSeq) return
    stats.value = result
  } catch (error) {
    if (seq !== requestSeq) return
    console.error('Failed to load live usage stats:', error)
    stats.value = null
  } finally {
    if (seq === requestSeq) loaded.value = true
  }
}

const stopPolling = () => {
  if (timer) clearInterval(timer)
  timer = null
}

const startPolling = () => {
  stopPolling()
  timer = setInterval(() => void load(), POLL_INTERVAL_MS)
}

// 页面隐藏时暂停轮询，回到页面时立即刷新一次再恢复。
const onVisibilityChange = () => {
  if (document.visibilityState === 'hidden') {
    stopPolling()
    return
  }
  void load()
  startPolling()
}

onMounted(() => {
  void load()
  if (document.visibilityState !== 'hidden') startPolling()
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onBeforeUnmount(() => {
  stopPolling()
  document.removeEventListener('visibilitychange', onVisibilityChange)
})

// 供仪表盘刷新按钮联动调用。
defineExpose({ reload: load })
</script>
