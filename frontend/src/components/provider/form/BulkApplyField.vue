<template>
  <div :class="plain ? 'space-y-2' : 'settings-section space-y-3'">
    <div class="flex items-start gap-3">
      <input
        :id="`${id}-enabled`"
        v-model="apply"
        type="checkbox"
        class="mt-0.5 h-4 w-4 shrink-0"
        :aria-controls="bodyId"
        :data-testid="applyTestid"
      />
      <div class="min-w-0 flex-1">
        <label
          :id="`${id}-label`"
          :for="`${id}-enabled`"
          class="cursor-pointer text-sm font-medium text-primary-900 dark:text-dark-50"
        >{{ label }}</label>
        <p v-if="hint" class="input-hint">{{ hint }}</p>
      </div>
      <div
        v-if="$slots.control"
        :id="$slots.default ? undefined : bodyId"
        class="shrink-0 transition-opacity"
        :class="!active && 'opacity-50'"
        :inert="!active || undefined"
      >
        <slot name="control" />
      </div>
    </div>
    <div
      v-if="$slots.default"
      :id="bodyId"
      role="group"
      :aria-labelledby="`${id}-label`"
      class="space-y-3 transition-opacity"
      :class="!active && 'opacity-50'"
      :inert="!active || undefined"
    >
      <slot />
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

// 批量编辑的每一项都由左侧“应用”复选框控制：未勾选时不提交，控件整体置灰且无法聚焦。
const props = defineProps<{
  /** 复选框 id 为 `${id}-enabled`，内容区 id 为 `${id}-body`。 */
  id: string
  label: string
  hint?: string
  /** 网格中的紧凑项，不参与分区分隔线。 */
  plain?: boolean
  /** 已勾选但依赖的其他设置不满足时，仍然锁定内容区。 */
  locked?: boolean
  applyTestid?: string
}>()
const apply = defineModel<boolean>({ required: true })
const bodyId = computed(() => `${props.id}-body`)
const active = computed(() => apply.value && !props.locked)
</script>
