<template>
  <li class="relative flex gap-4" :class="!last && 'pb-6'">
    <!-- 步骤之间的竖向连接线 -->
    <span
      v-if="!last"
      aria-hidden="true"
      class="absolute bottom-0 left-3.5 top-8 w-px bg-gray-200 dark:bg-dark-600"
    />
    <span
      class="relative flex h-7 w-7 shrink-0 items-center justify-center rounded-full text-xs font-semibold transition-colors"
      :class="done
        ? 'bg-primary-600 text-white'
        : 'border border-gray-300 bg-white text-gray-600 dark:border-dark-500 dark:bg-dark-900 dark:text-gray-300'"
    >
      <Icon v-if="done" name="check" size="xs" :stroke-width="2.5" :animate-on-hover="false" />
      <template v-else>{{ index }}</template>
    </span>
    <div class="min-w-0 flex-1 space-y-3 pt-1">
      <div>
        <p class="text-sm font-medium text-primary-900 dark:text-dark-50">{{ title }}</p>
        <p v-if="description" class="input-hint">{{ description }}</p>
      </div>
      <slot />
    </div>
  </li>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'

// 手动授权流程中的单个步骤：序号圆点、标题说明和步骤内容，完成后序号换成对勾。
defineProps<{
  index: number
  title: string
  description?: string
  done?: boolean
  /** 最后一步不绘制连接线。 */
  last?: boolean
}>()
</script>
