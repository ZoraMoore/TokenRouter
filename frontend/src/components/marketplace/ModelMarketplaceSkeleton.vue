<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Skeleton from '@/components/common/Skeleton.vue'

const { t } = useI18n()
</script>

<template>
  <!-- 与模型分组和模型卡片使用相同的网格断点。 -->
  <div class="space-y-4" role="status" :aria-label="t('common.loading')" aria-busy="true" data-testid="marketplace-skeleton">
    <section v-for="group in 2" :key="group" class="card overflow-hidden" aria-hidden="true">
      <div class="card-header space-y-3 px-4 py-4">
        <Skeleton :width="80" :height="20" />
        <div class="flex items-center gap-3">
          <Skeleton :width="36" :height="36" />
          <Skeleton :width="144" :height="20" />
        </div>
      </div>
      <div class="grid grid-cols-1 gap-3 p-4 md:grid-cols-2 lg:grid-cols-3">
        <div v-for="model in 3" :key="model" class="rounded-surface border border-gray-200 p-4 dark:border-dark-600">
          <Skeleton width="65%" :height="20" />
          <div class="mt-3 space-y-4">
            <Skeleton width="40%" :height="12" />
            <Skeleton :height="36" />
            <Skeleton width="50%" :height="12" />
          </div>
        </div>
      </div>
    </section>
  </div>
</template>
