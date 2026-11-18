<template>
  <div class="card">
    <!-- 卡头与公告卡保持同一结构：图标 + 标题 -->
    <div class="flex min-h-[61px] items-center gap-2.5 border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <Icon name="bolt" size="md" class="shrink-0 text-primary-600 dark:text-primary-400" />
      <h2 class="truncate text-lg font-semibold text-gray-900 dark:text-dark-50">{{ t('dashboard.quickActions') }}</h2>
    </div>
    <div class="space-y-2 p-4">
      <button @click="router.push('/keys')" class="group flex w-full items-center gap-4 rounded-control bg-gray-50 p-4 text-left transition duration-normal hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800">
        <div class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-surface bg-primary-100 transition-transform group-hover:scale-105 dark:bg-primary-900/30">
          <Icon name="key" size="lg" class="text-primary-600 dark:text-primary-400" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-gray-900 dark:text-dark-50">{{ t('dashboard.createApiKey') }}</p>
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.generateNewKey') }}</p>
        </div>
        <Icon
          name="chevronRight"
          size="sm"
          class="text-gray-400 transition duration-fast group-hover:translate-x-0.5 group-hover:text-primary-500 dark:text-dark-500"
          :animate-on-hover="false"
        />
      </button>

      <button @click="router.push('/usage')" class="group flex w-full items-center gap-4 rounded-control bg-gray-50 p-4 text-left transition duration-normal hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800">
        <div class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-surface bg-emerald-100 transition-transform group-hover:scale-105 dark:bg-emerald-900/30">
          <Icon name="chart" size="lg" class="text-emerald-600 dark:text-emerald-400" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-gray-900 dark:text-dark-50">{{ t('dashboard.viewUsage') }}</p>
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.checkDetailedLogs') }}</p>
        </div>
        <Icon
          name="chevronRight"
          size="sm"
          class="text-gray-400 transition duration-fast group-hover:translate-x-0.5 group-hover:text-emerald-500 dark:text-dark-500"
          :animate-on-hover="false"
        />
      </button>

      <button v-if="canUseBatchImage" @click="router.push('/batch-image')" class="group flex w-full items-center gap-4 rounded-control bg-gray-50 p-4 text-left transition duration-normal hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800">
        <div class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-surface bg-sky-100 transition-transform group-hover:scale-105 dark:bg-sky-900/30">
          <Icon name="sparkles" size="lg" class="text-sky-600 dark:text-sky-400" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-gray-900 dark:text-dark-50">{{ t('dashboard.batchImageAgent') }}</p>
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.batchImageAgentDesc') }}</p>
        </div>
        <Icon
          name="chevronRight"
          size="sm"
          class="text-gray-400 transition duration-fast group-hover:translate-x-0.5 group-hover:text-sky-500 dark:text-dark-500"
          :animate-on-hover="false"
        />
      </button>

      <button
        v-if="paymentEnabled"
        data-testid="purchase-quick-action"
        @click="router.push('/purchase')"
        class="group flex w-full items-center gap-4 rounded-control bg-gray-50 p-4 text-left transition duration-normal hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800"
      >
        <div class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-surface bg-rose-100 transition-transform group-hover:scale-105 dark:bg-rose-900/30">
          <Icon name="creditCard" size="lg" class="text-rose-600 dark:text-rose-400" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-gray-900 dark:text-dark-50">{{ t('nav.buySubscription') }}</p>
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.purchasePlanOrRecharge') }}</p>
        </div>
        <Icon
          name="chevronRight"
          size="sm"
          class="text-gray-400 transition duration-fast group-hover:translate-x-0.5 group-hover:text-rose-500 dark:text-dark-500"
          :animate-on-hover="false"
        />
      </button>

      <button @click="router.push('/redeem')" class="group flex w-full items-center gap-4 rounded-control bg-gray-50 p-4 text-left transition duration-normal hover:bg-gray-100 dark:bg-dark-800/50 dark:hover:bg-dark-800">
        <div class="flex h-12 w-12 flex-shrink-0 items-center justify-center rounded-surface bg-amber-100 transition-transform group-hover:scale-105 dark:bg-amber-900/30">
          <Icon name="gift" size="lg" class="text-amber-600 dark:text-amber-400" />
        </div>
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-gray-900 dark:text-dark-50">{{ t('dashboard.redeemCode') }}</p>
          <p class="text-xs text-gray-500 dark:text-dark-400">{{ t('dashboard.addBalanceWithCode') }}</p>
        </div>
        <Icon
          name="chevronRight"
          size="sm"
          class="text-gray-400 transition duration-fast group-hover:translate-x-0.5 group-hover:text-amber-500 dark:text-dark-500"
          :animate-on-hover="false"
        />
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useBatchImageAccess } from '@/composables/useBatchImageAccess'
import { useAppStore } from '@/stores/app'

const router = useRouter()
const { t } = useI18n()
const appStore = useAppStore()
const { canUseBatchImage, refreshBatchImageAccess } = useBatchImageAccess()
// 公共设置开启支付后展示入口，加载失败时保持隐藏。
const paymentEnabled = computed(() => appStore.cachedPublicSettings?.payment_enabled === true)

onMounted(() => {
  void refreshBatchImageAccess()
})
</script>
