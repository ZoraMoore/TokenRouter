<template>
  <p v-if="version || formattedUpdatedAt" class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-gray-500 dark:text-dark-400">
    <span v-if="version" :title="version">models.dev · {{ version.slice(0, 12) }}</span>
    <span v-if="formattedUpdatedAt">{{ t('admin.pricing.defaults.updatedAt') }} {{ formattedUpdatedAt }}</span>
  </p>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

const props = defineProps<{
  version: string
  updatedAt: string
}>()

const { t } = useI18n()

// 价格与属性读取同一目录，统一省略尚未成功更新时的空时间和零值时间。
const formattedUpdatedAt = computed(() => {
  if (!props.updatedAt || props.updatedAt.startsWith('0001')) return ''
  const date = new Date(props.updatedAt)
  return Number.isNaN(date.getTime()) ? '' : date.toLocaleString()
})
</script>
