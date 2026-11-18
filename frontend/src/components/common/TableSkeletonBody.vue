<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import Skeleton from './Skeleton.vue'

// 骨架使用表头的列宽，调用方传入当前可见列数。
withDefaults(defineProps<{
  columns: number
  rows?: number
  cellClass?: string
}>(), {
  rows: 5,
  cellClass: 'px-4 py-3'
})

const { t } = useI18n()
</script>

<template>
  <tbody :aria-label="t('common.loading')" aria-busy="true" data-loading-skeleton>
    <tr v-for="row in rows" :key="row" aria-hidden="true">
      <td v-for="column in columns" :key="column" :class="cellClass">
        <Skeleton :width="column % 2 ? '80%' : '60%'" :height="16" />
      </td>
    </tr>
  </tbody>
</template>
