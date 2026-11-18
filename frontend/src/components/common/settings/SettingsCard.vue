<template>
  <section class="card">
    <header class="card-header flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0 flex-1">
        <h2 class="text-lg font-semibold text-primary-900 dark:text-dark-50">
          {{ title }}
        </h2>
        <p
          v-if="description"
          class="mt-1 text-sm text-primary-900/80 dark:text-dark-300"
        >
          {{ description }}
        </p>
        <slot name="description" />
      </div>
      <slot name="actions" />
    </header>
    <div class="space-y-6 p-6">
      <slot />
    </div>
  </section>
</template>

<script setup lang="ts">
// 整页设置的一组设置项放在一张卡片里：头部是标题、说明和 actions 插槽，内容区按 SettingsSection 分区。
// 保存统一由页面底部的吸底保存条完成，卡片里只放刷新、测试这类操作按钮。
defineProps<{
  title: string
  description?: string
}>()
</script>
