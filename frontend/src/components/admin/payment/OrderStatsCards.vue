<template>
  <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 xl:grid-cols-5">
    <!-- Today Revenue -->
    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-green-100 p-2 dark:bg-green-900/30">
          <Icon name="dollar" size="md" class="text-green-600 dark:text-green-400" :stroke-width="2" />
        </div>
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.todayRevenue') }}</p>
          <p class="text-xl font-bold text-gray-900 dark:text-white">${{ formatMoney(stats.today_amount) }}</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ stats.today_count }} {{ t('payment.admin.orders') }}
          </p>
        </div>
      </div>
    </div>

    <!-- Total Revenue -->
    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-blue-100 p-2 dark:bg-blue-900/30">
          <Icon name="creditCard" size="md" class="text-blue-600 dark:text-blue-400" :stroke-width="2" />
        </div>
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.totalRevenue') }}</p>
          <p class="text-xl font-bold text-gray-900 dark:text-white">${{ formatMoney(stats.total_amount) }}</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ stats.total_count }} {{ t('payment.admin.orders') }}
          </p>
        </div>
      </div>
    </div>

    <!-- Today Orders -->
    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-purple-100 p-2 dark:bg-purple-900/30">
          <Icon name="chart" size="md" class="text-purple-600 dark:text-purple-400" :stroke-width="2" />
        </div>
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.todayOrders') }}</p>
          <p class="text-xl font-bold text-gray-900 dark:text-white">{{ stats.today_count }}</p>
        </div>
      </div>
    </div>

    <!-- Average Amount -->
    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-amber-100 p-2 dark:bg-amber-900/30">
          <Icon name="chart" size="md" class="text-amber-600 dark:text-amber-400" :stroke-width="2" />
        </div>
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.avgAmount') }}</p>
          <p class="text-xl font-bold text-gray-900 dark:text-white">${{ formatMoney(stats.avg_amount) }}</p>
        </div>
      </div>
    </div>

    <!-- 平均余额单位购买单价 -->
    <div class="card p-4">
      <div class="flex items-center gap-3">
        <div class="rounded-lg bg-sky-100 p-2 dark:bg-sky-900/30">
          <Icon name="brain" size="md" class="text-sky-600 dark:text-sky-400" :stroke-width="2" />
        </div>
        <div>
          <p class="text-xs font-medium text-gray-500 dark:text-gray-400">{{ t('payment.admin.avgBalanceUnitPurchaseUnitPrice', { unitName: balanceUnitName }) }}</p>
          <p class="text-xl font-bold text-gray-900 dark:text-white">&yen;{{ formatUnitPrice(stats.avg_reasoning_point_purchase_unit_price) }}/{{ balanceUnitName }}</p>
          <p class="text-xs text-gray-500 dark:text-gray-400">
            {{ stats.reasoning_point_purchase_order_count }} {{ t('payment.admin.orders') }}
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import Icon from '@/components/icons/Icon.vue'
import type { DashboardStats } from '@/types/payment'

const { t } = useI18n()
const { balanceUnitName } = useBalanceDisplay()

defineProps<{
  stats: DashboardStats
}>()

function formatMoney(value: number): string {
  return value.toFixed(2)
}

function formatUnitPrice(value: number): string {
  return value.toFixed(4)
}
</script>
