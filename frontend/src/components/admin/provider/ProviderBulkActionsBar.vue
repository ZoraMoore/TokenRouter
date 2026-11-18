<template>
  <!-- 独立于表格卡片的工具条，只在勾选提供商后出现，放在筛选区最后一行。 -->
  <div
    v-if="hasSelection"
    class="flex flex-wrap items-center justify-between gap-2 rounded-surface border border-gray-200 bg-white px-4 py-2 dark:border-dark-600 dark:bg-dark-900"
  >
    <div class="flex min-w-0 flex-wrap items-center gap-x-3 gap-y-1 text-sm">
      <span class="font-medium text-gray-900 dark:text-gray-100">
        <template v-if="allResultsSelected">
          {{ t('admin.providers.bulkActions.selectedAll', { count: selectedIds.length }) }}
        </template>
        <template v-else>
          {{ t('admin.providers.bulkActions.selected', { count: selectedIds.length }) }}
        </template>
      </span>
      <span class="h-4 w-px bg-gray-200 dark:bg-dark-600" aria-hidden="true"></span>
      <button
        type="button"
        class="bulk-bar-link"
        @click="$emit('select-page')"
      >
        {{ t('admin.providers.bulkActions.selectCurrentPage') }}
      </button>
      <button
        v-if="!allResultsSelected && totalResults > selectedIds.length"
        type="button"
        class="bulk-bar-link"
        :disabled="selectingAll"
        @click="$emit('select-all-results')"
      >
        {{
          selectingAll
            ? t('admin.providers.bulkActions.selectingAll')
            : t('admin.providers.bulkActions.selectAllResults', { count: totalResults })
        }}
      </button>
      <button
        type="button"
        class="bulk-bar-link"
        @click="$emit('clear')"
      >
        {{ t('admin.providers.bulkActions.clear') }}
      </button>
    </div>

    <!-- 只露出高频操作，低频操作与删除收进「更多」菜单，按钮统一用中性样式。 -->
    <div class="flex flex-wrap justify-end gap-2">
      <button type="button" class="btn btn-secondary btn-sm" @click="$emit('toggle-schedulable', true)">
        <Icon name="play" size="sm" />
        {{ t('admin.providers.bulkActions.enableScheduling') }}
      </button>
      <button type="button" class="btn btn-secondary btn-sm" @click="$emit('toggle-schedulable', false)">
        <Icon name="ban" size="sm" />
        {{ t('admin.providers.bulkActions.disableScheduling') }}
      </button>
      <button type="button" class="btn btn-secondary btn-sm" @click="$emit('edit-selected')">
        <Icon name="edit" size="sm" />
        {{ t('admin.providers.bulkActions.edit') }}
      </button>
      <button
        ref="moreButtonRef"
        type="button"
        class="btn btn-secondary btn-sm"
        :aria-expanded="moreMenuOpen"
        aria-haspopup="menu"
        @click="toggleMoreMenu"
      >
        {{ t('admin.providers.bulkActions.more') }}
        <Icon name="chevronDown" size="sm" />
      </button>
    </div>

    <Teleport to="body">
      <template v-if="moreMenuOpen && menuPosition">
        <div class="fixed inset-0 z-menu-overlay" @click="closeMoreMenu"></div>
        <div
          class="dropdown fixed z-action-menu py-0"
          role="menu"
          :style="{ top: `${menuPosition.top}px`, left: `${menuPosition.left}px`, width: `${menuPosition.width}px` }"
        >
          <div class="menu-section">
            <button type="button" role="menuitem" class="menu-item" @click="runMenuAction('reset-status')">
              <Icon name="undo" size="sm" />
              {{ t('admin.providers.bulkActions.resetStatus') }}
            </button>
            <button type="button" role="menuitem" class="menu-item" @click="runMenuAction('refresh-token')">
              <Icon name="key" size="sm" />
              {{ t('admin.providers.bulkActions.refreshToken') }}
            </button>
            <button
              type="button"
              role="menuitem"
              class="menu-item disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="usageLoading"
              @click="runMenuAction('query-usage')"
            >
              <Icon name="chartBar" size="sm" />
              {{ t('admin.providers.bulkActions.queryUsage') }}
            </button>
            <button
              type="button"
              role="menuitem"
              class="menu-item disabled:cursor-not-allowed disabled:opacity-60"
              :disabled="upstreamUsageLoading"
              @click="runMenuAction('query-upstream-usage')"
            >
              <Icon name="cloud" size="sm" />
              {{ t('admin.providers.bulkActions.queryUpstreamUsage') }}
            </button>
          </div>
          <div class="menu-section">
            <button type="button" role="menuitem" class="menu-item menu-item-danger" @click="runMenuAction('delete')">
              <Icon name="trash" size="sm" />
              {{ t('admin.providers.bulkActions.delete') }}
            </button>
          </div>
        </div>
      </template>
    </Teleport>
  </div>
</template>

<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { getFloatingPanelPosition, type FloatingPanelPosition } from '@/utils/floatingPanel'

type MenuAction = 'reset-status' | 'refresh-token' | 'query-usage' | 'query-upstream-usage' | 'delete'

// 「更多」菜单宽度与预估高度，供浮层定位判断是否需要向上翻转。
const MORE_MENU_WIDTH = 208
const MORE_MENU_HEIGHT = 240

const props = defineProps<{
  selectedIds: number[]
  usageLoading?: boolean
  upstreamUsageLoading?: boolean
  totalResults: number
  selectingAll: boolean
  allResultsSelected: boolean
}>()

const emit = defineEmits<{
  delete: []
  'edit-selected': []
  clear: []
  'select-page': []
  'select-all-results': []
  'toggle-schedulable': [schedulable: boolean]
  'reset-status': []
  'refresh-token': []
  'query-usage': []
  'query-upstream-usage': []
}>()

const { t } = useI18n()

const hasSelection = computed(() => props.selectedIds.length > 0)

const moreButtonRef = ref<HTMLButtonElement | null>(null)
const moreMenuOpen = ref(false)
const menuPosition = ref<FloatingPanelPosition | null>(null)

const closeMoreMenu = () => {
  moreMenuOpen.value = false
}

const toggleMoreMenu = () => {
  if (moreMenuOpen.value || !moreButtonRef.value) {
    closeMoreMenu()
    return
  }
  menuPosition.value = getFloatingPanelPosition(
    moreButtonRef.value.getBoundingClientRect(),
    window.innerWidth,
    window.innerHeight,
    { maxWidth: MORE_MENU_WIDTH, fixedHeight: MORE_MENU_HEIGHT, pinLeftOnMobile: false }
  )
  moreMenuOpen.value = true
}

// 菜单动作先关闭浮层，再显示确认弹窗。
const runMenuAction = (action: MenuAction) => {
  closeMoreMenu()
  if (action === 'reset-status') emit('reset-status')
  else if (action === 'refresh-token') emit('refresh-token')
  else if (action === 'query-usage') emit('query-usage')
  else if (action === 'query-upstream-usage') emit('query-upstream-usage')
  else emit('delete')
}

const handleEscape = (event: KeyboardEvent) => {
  if (event.key === 'Escape') closeMoreMenu()
}

watch(moreMenuOpen, (open) => {
  if (open) window.addEventListener('keydown', handleEscape)
  else window.removeEventListener('keydown', handleEscape)
})

// 选中项被清空时整条工具栏随之消失，菜单也要一起收起。
watch(hasSelection, (selected) => {
  if (!selected) closeMoreMenu()
})

onUnmounted(() => window.removeEventListener('keydown', handleEscape))
</script>

<style scoped>
/* 工具栏内的文字操作：常态为品牌色，悬停加下划线，禁用时降低不透明度。 */
.bulk-bar-link {
  @apply text-primary-600 underline-offset-4 transition-colors duration-fast hover:text-primary-700 hover:underline disabled:cursor-not-allowed disabled:no-underline disabled:opacity-60 dark:text-primary-400 dark:hover:text-primary-300;
}
</style>
