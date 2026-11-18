<template>
  <div
    class="flex justify-between gap-4"
    :class="layoutClass"
    :data-setting-row="setting"
  >
    <div class="min-w-0">
      <div class="flex items-center gap-2">
        <label
          v-if="labelFor"
          :for="labelFor"
          class="text-sm font-medium text-primary-900 dark:text-dark-50"
        >{{ label }}</label>
        <span
          v-else
          class="text-sm font-medium text-primary-900 dark:text-dark-50"
        >{{ label }}</span>
        <HelpTooltip
          v-if="help"
          :content="help"
          :tooltip-id="`${id}-help`"
          trigger="both"
          :closable="false"
        >
          <template #trigger>
            <button
              type="button"
              class="inline-flex rounded-compact text-gray-400 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary-500"
              :aria-label="label"
              :aria-describedby="`${id}-help`"
            >
              <Icon name="questionCircle" size="sm" />
            </button>
          </template>
        </HelpTooltip>
      </div>
      <p v-if="hint" :id="`${id}-hint`" class="input-hint">{{ hint }}</p>
      <slot name="hint" />
    </div>
    <div class="shrink-0" :class="field && 'w-full sm:w-56'">
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, useSlots } from 'vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import Icon from '@/components/icons/Icon.vue'

// 设置行统一左侧标题说明、右侧控件的布局；field 用于右侧放选择框或输入框的行，窄屏改为上下排列。
const props = defineProps<{
  id: string
  label: string
  hint?: string
  help?: string
  setting?: string
  /** 标题关联的控件 id，选择框和输入框行传入以便点击标题聚焦。 */
  labelFor?: string
  field?: boolean
}>()

const slots = useSlots()

// 没有说明文字时，单行标题和控件垂直居中；有说明时控件和标题顶部对齐。
// 输入框行在窄屏上下排列，只在 sm 及以上应用对齐方式。
const layoutClass = computed(() => {
  const hasHint = Boolean(props.hint || slots.hint)
  if (!props.field) return hasHint ? 'items-start' : 'items-center'
  return hasHint
    ? 'flex-col gap-y-2 sm:flex-row sm:items-start'
    : 'flex-col gap-y-2 sm:flex-row sm:items-center'
})
</script>
