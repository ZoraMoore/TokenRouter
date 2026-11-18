<template>
  <span class="badge whitespace-nowrap" :class="probe && status === 'suspected' ? 'badge-warning' : colors[status]">
    {{ t(probe ? `pelicanTests.probe.status.${status}` : `pelicanTests.status.${status}`) }}
  </span>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import type { PelicanStatus } from '@/api/pelicanTests'

defineProps<{ status: PelicanStatus; probe?: boolean }>()
const { t } = useI18n()

// 待复核样本和无法判断的样本分别使用提示色和中性色。
const colors: Record<PelicanStatus, string> = {
  pending_review: 'badge-warning',
  passed: 'badge-success',
  suspected: 'badge-danger',
  inconclusive: 'badge-gray',
}
</script>
