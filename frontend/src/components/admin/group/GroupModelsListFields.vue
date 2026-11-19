<template>
  <SettingsSection data-group-field="models-list">
    <SettingToggleRow
      :id="`${idPrefix}-models-list`"
      :model-value="state.enabled"
      :label="t('admin.groups.modelsList.title', { endpoint: '/v1/models' })"
      :hint="t('admin.groups.modelsList.hint', { endpoint: '/v1/models' })"
      setting="enabled"
      @update:model-value="emit('enabled', $event)"
    />
    <div
      v-if="state.enabled"
      class="overflow-hidden rounded-surface border border-gray-200 dark:border-dark-600"
    >
      <div v-if="status === 'error'" class="flex flex-wrap items-center gap-2 p-4">
        <p role="alert" class="input-hint">{{ t('admin.groups.modelsList.loadFailed') }}</p>
        <button type="button" class="btn btn-secondary btn-sm" @click="emit('retry')">
          {{ t('common.retry') }}
        </button>
      </div>
      <p v-if="status === 'loading'" role="status" class="input-hint p-4">
        {{ t('admin.groups.modelsList.loading') }}
      </p>
      <p v-if="status === 'ready' && !items.length" class="input-hint p-4">
        {{ t('admin.groups.modelsList.empty') }}
      </p>
      <div
        v-if="status === 'ready' && items.length"
        class="flex flex-wrap items-center justify-between gap-2 border-b border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800"
      >
        <span class="text-xs text-gray-500 dark:text-gray-400">{{
          t('admin.groups.settings.selectedModels', {
            selected: selectedCount,
            total: items.length,
          })
        }}</span>
        <div class="flex gap-2">
          <button type="button" class="btn btn-secondary" @click="emit('selectAll')">
            {{ t('admin.groups.settings.selectAll') }}
          </button>
          <button type="button" class="btn btn-secondary" @click="emit('invert')">
            {{ t('admin.groups.settings.invertSelection') }}
          </button>
        </div>
      </div>
      <div v-if="items.length" ref="listRef" class="max-h-64 overflow-y-auto">
        <div class="relative" :style="{ height: `${listVirtualizer.getTotalSize()}px` }">
          <div
            v-for="row in visibleRows"
            :key="items[row.index].id"
            :ref="measureRow"
            :data-index="row.index"
            :style="{ transform: `translateY(${row.start}px)` }"
            class="absolute inset-x-0 top-0 flex items-center gap-2 border-b border-gray-200 px-4 py-3 dark:border-dark-600"
            @keydown="onRowKeydown($event, row.index)"
          >
            <span class="min-w-0 flex-1 break-all text-sm text-primary-900 dark:text-dark-50">
              {{ items[row.index].id }}
            </span>
            <Toggle
              :model-value="items[row.index].selected"
              :aria-label="items[row.index].id"
              :data-model-visibility="items[row.index].id"
              data-model-focus="toggle"
              :disabled="status !== 'ready'"
              size="md"
              class="shrink-0"
              @update:model-value="emit('select', items[row.index].id, $event)"
            />
            <div class="flex shrink-0">
              <button
                type="button"
                class="btn btn-ghost btn-icon"
                data-model-focus="up"
                :disabled="status !== 'ready' || row.index === 0"
                :aria-label="t('admin.groups.settings.moveUp', { name: items[row.index].id })"
                @click="move($event, row.index, -1)"
              >
                <Icon name="arrowUp" size="sm" />
              </button>
              <button
                type="button"
                class="btn btn-ghost btn-icon"
                data-model-focus="down"
                :disabled="status !== 'ready' || row.index === items.length - 1"
                :aria-label="t('admin.groups.settings.moveDown', { name: items[row.index].id })"
                @click="move($event, row.index, 1)"
              >
                <Icon name="arrowDown" size="sm" />
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </SettingsSection>
</template>

<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import { observeElementRect, useVirtualizer, type Rect, type Virtualizer } from '@tanstack/vue-virtual'
import { useI18n } from 'vue-i18n'
import Toggle from '@/components/common/Toggle.vue'
import Icon from '@/components/icons/Icon.vue'
import SettingToggleRow from '@/components/common/settings/SettingToggleRow.vue'
import SettingsSection from '@/components/common/settings/SettingsSection.vue'
import type { ModelsListState } from '@/views/admin/groupsModelsList'
import type { ModelsListCandidatesStatus } from '@/views/admin/groupsModelsListCandidates'

const props = defineProps<{
  idPrefix: string
  state: ModelsListState
  status: ModelsListCandidatesStatus
}>()
const emit = defineEmits<{
  enabled: [value: boolean]
  select: [id: string, value: boolean]
  selectAll: []
  invert: []
  move: [from: number, to: number]
  retry: []
}>()
const { t } = useI18n()

// 候选到达前显示已保存的型号。选中数和排序下标来自整份清单。
const items = computed(() => {
  if (!props.state.enabled) return []
  if (props.status === 'ready') return props.state.items
  return props.state.savedModels.map(id => ({ id, selected: true }))
})
const selectedCount = computed(() => items.value.filter(item => item.selected).length)
const listRef = ref<HTMLElement | null>(null)
const estimatedRowHeight = 61
const listVirtualizer = useVirtualizer<HTMLElement, HTMLElement>(computed(() => {
  const rows = items.value
  return {
    count: rows.length,
    getScrollElement: () => listRef.value,
    getItemKey: (index: number) => rows[index].id,
    estimateSize: () => estimatedRowHeight,
    initialRect: { width: 0, height: 256 },
    observeElementRect: observeListSize,
    overscan: 4,
  }
}))
const visibleRows = computed(() => listVirtualizer.value.getVirtualItems())

// 隐藏页签会报告零高，列表等到下一次可见布局再更新视窗。
function observeListSize(instance: Virtualizer<HTMLElement, HTMLElement>, callback: (size: Rect) => void) {
  return observeElementRect(instance, size => {
    if (size.height > 0) callback(size)
  })
}

function measureRow(element: unknown) {
  if (element instanceof HTMLElement) listVirtualizer.value.measureElement(element)
}

let pendingFocus: { index: number; control: string } | undefined

// 滚动事件更新虚拟行后再聚焦，焦点不会落到弹窗内的其他控件上。
function restoreRowFocus() {
  if (!pendingFocus) return
  const row = listRef.value?.querySelector(`[data-index="${pendingFocus.index}"]`)
  if (!row) return
  const control = row.querySelector<HTMLElement>(`[data-model-focus="${pendingFocus.control}"]:not([disabled])`)
    ?? row.querySelector<HTMLElement>('[data-model-focus="toggle"]:not([disabled])')
  control?.focus()
  pendingFocus = undefined
}

watch(visibleRows, restoreRowFocus, { flush: 'post' })

function focusRow(index: number, control: string) {
  pendingFocus = { index, control }
  listVirtualizer.value.scrollToIndex(index, { align: 'auto' })
  void nextTick(restoreRowFocus)
}

function onRowKeydown(event: KeyboardEvent, index: number) {
  if (event.key !== 'Tab' || props.status !== 'ready' || !(event.target instanceof HTMLElement)) return
  const backward = event.shiftKey
  const control = event.target.dataset.modelFocus
  if (control !== (backward ? 'toggle' : 'down')) return
  const next = index + (backward ? -1 : 1)
  if (next < 0 || next >= items.value.length) return
  if (listRef.value?.querySelector(`[data-index="${next}"]`)) return
  event.preventDefault()
  focusRow(next, backward ? 'down' : 'toggle')
}

function move(event: MouseEvent, index: number, offset: number) {
  const destination = index + offset
  emit('move', index, destination)
  if (event.detail === 0) {
    void nextTick(() => focusRow(destination, offset < 0 ? 'up' : 'down'))
  }
}
</script>
