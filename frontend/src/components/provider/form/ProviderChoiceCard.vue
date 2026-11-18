<template>
  <button
    type="button"
    role="radio"
    :aria-checked="selected"
    :disabled="disabled"
    class="flex w-full items-start gap-3 rounded-control border p-3 text-left transition-colors disabled:cursor-not-allowed disabled:opacity-60"
    :class="
      selected
        ? platformChoiceSelectedClass(accent ?? '')
        : 'border-gray-200 bg-white hover:border-gray-300 hover:bg-gray-50 dark:border-dark-600 dark:bg-dark-900 dark:hover:border-dark-500 dark:hover:bg-dark-800'
    "
  >
    <span
      class="flex h-8 w-8 shrink-0 items-center justify-center rounded-control"
      :class="selected ? platformSolidClass(accent ?? '') : 'bg-gray-100 text-gray-500 dark:bg-dark-700 dark:text-gray-400'"
    >
      <PlatformIcon v-if="platform" :platform="platform" size="sm" />
      <Icon v-else-if="icon" :name="icon" size="sm" :animate-on-hover="false" />
    </span>
    <span class="min-w-0 flex-1">
      <span class="block text-sm font-medium text-primary-900 dark:text-dark-50">{{ title }}</span>
      <span v-if="description" class="block text-xs text-gray-500 dark:text-gray-400">{{ description }}</span>
      <slot />
    </span>
    <slot name="aside" />
  </button>
</template>

<script setup lang="ts">
import PlatformIcon from '@/components/common/PlatformIcon.vue'
import Icon from '@/components/icons/Icon.vue'
import type { IconName } from '@/components/icons/registry'
import type { ProviderPlatform } from '@/types'
import { platformChoiceSelectedClass, platformSolidClass } from '@/utils/platformColors'

// 账号类型、接入模式等多选一卡片：结构统一，选中态按所属平台取品牌色，便于区分当前平台。
defineProps<{
  selected: boolean
  title: string
  description?: string
  icon?: IconName
  /** 传入时显示平台标识，优先于 icon。 */
  platform?: ProviderPlatform
  disabled?: boolean
  /** 所属平台，决定选中态的品牌色；未传时使用主题色。 */
  accent?: string
}>()
</script>
