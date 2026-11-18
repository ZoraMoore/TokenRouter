<template>
  <section class="settings-section space-y-4">
    <div v-if="title" class="flex flex-wrap items-end justify-between gap-3">
      <div class="min-w-0 flex-1">
        <h4 class="text-sm font-semibold text-primary-900 dark:text-dark-50">
          {{ title }}
        </h4>
        <p v-if="hint" class="input-hint">{{ hint }}</p>
      </div>
      <slot name="actions" />
    </div>
    <slot />
  </section>
</template>

<script setup lang="ts">
// 分区只提供标题和间距，复杂规则的边框与背景由内部编辑器或子面板负责。
defineProps<{ title?: string; hint?: string }>()
</script>

<style>
/* 不加 scoped：调用方用同名类包裹的自定义分区也参与分隔线规则。 */
.settings-section + .settings-section {
  @apply border-t border-gray-200 pt-6 dark:border-dark-600;
}
</style>
