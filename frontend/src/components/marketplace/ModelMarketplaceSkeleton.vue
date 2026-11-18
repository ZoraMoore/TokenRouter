<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Skeleton from '@/components/common/Skeleton.vue'

const { t } = useI18n()
</script>

<template>
  <!-- 窄屏的分组选择框和宽屏的左栏分别在控件位置占位。 -->
  <div
    class="grid min-w-0 items-start gap-4 md:grid-cols-[12rem_minmax(0,1fr)] xl:grid-cols-[15rem_minmax(0,1fr)]"
    role="status"
    :aria-label="t('common.loading')"
    aria-busy="true"
    data-testid="marketplace-skeleton"
  >
    <div class="card space-y-3 p-4" aria-hidden="true">
      <Skeleton width="50%" :height="20" />
      <Skeleton class="md:hidden" :height="36" />
      <div class="hidden gap-1 md:grid">
        <Skeleton v-for="item in 4" :key="item" :height="36" />
      </div>
    </div>
    <section class="card min-w-0" aria-hidden="true">
      <div class="card-header space-y-3 p-4 md:px-5">
        <div class="flex items-center gap-3">
          <Skeleton :width="48" :height="48" />
          <div class="flex-1 space-y-2">
            <Skeleton width="40%" :height="20" />
            <Skeleton width="60%" :height="16" />
          </div>
        </div>
        <div class="flex gap-2">
          <Skeleton :width="72" :height="24" />
          <Skeleton :width="96" :height="24" />
        </div>
      </div>
      <div class="flex items-center gap-2 px-4 pt-4 md:px-5">
        <Skeleton width="50%" :height="36" />
        <Skeleton :width="36" :height="36" />
      </div>
      <div class="grid grid-cols-[repeat(auto-fill,minmax(min(100%,17rem),1fr))] gap-4 p-4 md:px-5">
        <div v-for="model in 3" :key="model" class="min-w-0 rounded-surface border border-gray-200 p-4 dark:border-dark-600">
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
