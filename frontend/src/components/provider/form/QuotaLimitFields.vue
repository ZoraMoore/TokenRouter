<template>
  <SettingsSection :title="t('admin.providers.quotaControl.title')" :hint="hint">
    <QuotaLimitCard
      :total-limit="limits.totalLimit"
      :daily-limit="limits.dailyLimit"
      :weekly-limit="limits.weeklyLimit"
      :daily-reset-mode="limits.dailyResetMode"
      :daily-reset-hour="limits.dailyResetHour"
      :weekly-reset-mode="limits.weeklyResetMode"
      :weekly-reset-day="limits.weeklyResetDay"
      :weekly-reset-hour="limits.weeklyResetHour"
      :reset-timezone="limits.resetTimezone"
      :quota-notify-global-enabled="notifyGlobalEnabled"
      :quota-notify-daily-enabled="notify.daily.enabled"
      :quota-notify-daily-threshold="notify.daily.threshold"
      :quota-notify-daily-threshold-type="notify.daily.thresholdType"
      :quota-notify-weekly-enabled="notify.weekly.enabled"
      :quota-notify-weekly-threshold="notify.weekly.threshold"
      :quota-notify-weekly-threshold-type="notify.weekly.thresholdType"
      :quota-notify-total-enabled="notify.total.enabled"
      :quota-notify-total-threshold="notify.total.threshold"
      :quota-notify-total-threshold-type="notify.total.thresholdType"
      @update:total-limit="emit('update:limit', 'totalLimit', $event)"
      @update:daily-limit="emit('update:limit', 'dailyLimit', $event)"
      @update:weekly-limit="emit('update:limit', 'weeklyLimit', $event)"
      @update:daily-reset-mode="emit('update:limit', 'dailyResetMode', $event)"
      @update:daily-reset-hour="emit('update:limit', 'dailyResetHour', $event)"
      @update:weekly-reset-mode="emit('update:limit', 'weeklyResetMode', $event)"
      @update:weekly-reset-day="emit('update:limit', 'weeklyResetDay', $event)"
      @update:weekly-reset-hour="emit('update:limit', 'weeklyResetHour', $event)"
      @update:reset-timezone="emit('update:limit', 'resetTimezone', $event)"
      @update:quota-notify-daily-enabled="emit('update:notify', 'daily', 'enabled', $event)"
      @update:quota-notify-daily-threshold="emit('update:notify', 'daily', 'threshold', $event)"
      @update:quota-notify-daily-threshold-type="emit('update:notify', 'daily', 'thresholdType', $event)"
      @update:quota-notify-weekly-enabled="emit('update:notify', 'weekly', 'enabled', $event)"
      @update:quota-notify-weekly-threshold="emit('update:notify', 'weekly', 'threshold', $event)"
      @update:quota-notify-weekly-threshold-type="emit('update:notify', 'weekly', 'thresholdType', $event)"
      @update:quota-notify-total-enabled="emit('update:notify', 'total', 'enabled', $event)"
      @update:quota-notify-total-threshold="emit('update:notify', 'total', 'threshold', $event)"
      @update:quota-notify-total-threshold-type="emit('update:notify', 'total', 'thresholdType', $event)"
    />
  </SettingsSection>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import SettingsSection from '@/components/common/settings/SettingsSection.vue'
import type { QuotaNotifyDim, QuotaNotifyDimState } from '@/composables/useQuotaNotifyState'
import QuotaLimitCard from '../QuotaLimitCard.vue'
import type { QuotaLimitValues } from './quotaLimit'

// 额度限制与通知阈值的接线集中在这里，创建与编辑只维护各自的草稿。
defineProps<{
  limits: QuotaLimitValues
  notify: Record<QuotaNotifyDim, QuotaNotifyDimState>
  notifyGlobalEnabled: boolean
  hint?: string
}>()
const emit = defineEmits<{
  'update:limit': [key: keyof QuotaLimitValues, value: QuotaLimitValues[keyof QuotaLimitValues]]
  'update:notify': [
    dim: QuotaNotifyDim,
    field: keyof QuotaNotifyDimState,
    value: QuotaNotifyDimState[keyof QuotaNotifyDimState]
  ]
}>()

const { t } = useI18n()
</script>
