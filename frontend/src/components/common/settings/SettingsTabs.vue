<template>
  <div ref="rootRef" class="settings-tabs" @onboarding-reveal="onTourReveal">
    <div class="settings-tab-list" role="tablist" :aria-label="label">
      <button
        v-for="tab in visibleTabs"
        :id="`${idPrefix}-tab-${tab.key}`"
        :key="tab.key"
        type="button"
        role="tab"
        :aria-selected="activeTab === tab.key"
        :aria-controls="`${idPrefix}-panel-${tab.key}`"
        :tabindex="activeTab === tab.key ? 0 : -1"
        :data-settings-tab-button="tab.key"
        class="settings-tab"
        :class="{ 'settings-tab-active': activeTab === tab.key }"
        @click="selectTab(tab.key)"
        @keydown="onTabKeydown($event, tab.key)"
      >
        {{ tab.label }}
      </button>
    </div>
    <div ref="contentRef" class="settings-tab-content">
      <!-- 页签持续挂载，切页与暂时隐藏后编辑器仍保存草稿。 -->
      <section
        v-for="tab in tabs"
        v-show="isShown(tab.key)"
        v-content-reveal="isShown(tab.key)"
        :id="`${idPrefix}-panel-${tab.key}`"
        :key="tab.key"
        role="tabpanel"
        :aria-labelledby="`${idPrefix}-tab-${tab.key}`"
        :data-settings-tab="tab.key"
        tabindex="0"
        class="space-y-6"
      >
        <slot :name="tab.key" />
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { vContentReveal } from '@/directives/contentReveal'

import { computed, nextTick, ref, watch } from 'vue'

export interface SettingsTab {
  key: string
  label: string
  /** 隐藏的页签不显示按钮，但面板保持挂载。 */
  hidden?: boolean
}

const props = defineProps<{
  idPrefix: string
  tabs: SettingsTab[]
  /** 页签栏的可访问名称。 */
  label: string
}>()

const rootRef = ref<HTMLElement | null>(null)
const contentRef = ref<HTMLElement | null>(null)
const visibleTabs = computed(() => props.tabs.filter(tab => !tab.hidden))
const activeTab = ref(visibleTabs.value[0]?.key ?? '')

// 当前页签因平台或类型变化被隐藏时，回到第一个可见页签。
watch(visibleTabs, tabs => {
  if (!tabs.some(tab => tab.key === activeTab.value)) activeTab.value = tabs[0]?.key ?? ''
})

// 回顶在 DOM 更新后的同一刷新周期完成，先于 revealElement 的 nextTick 定位。
watch(activeTab, () => {
  if (contentRef.value) contentRef.value.scrollTop = 0
}, { flush: 'post' })

function isShown(key: string) {
  return activeTab.value === key && visibleTabs.value.some(tab => tab.key === key)
}

function findTabButton(key: string) {
  return rootRef.value?.querySelector<HTMLElement>(`[data-settings-tab-button="${key}"]`)
}

async function selectTab(key: string) {
  activeTab.value = key
  await nextTick()
  findTabButton(key)?.scrollIntoView?.({ block: 'nearest', inline: 'nearest' })
}

function onTabKeydown(event: KeyboardEvent, key: string) {
  const keys = visibleTabs.value.map(tab => tab.key)
  const index = keys.indexOf(key)
  let next: string | undefined
  if (event.key === 'ArrowRight') next = keys[(index + 1) % keys.length]
  if (event.key === 'ArrowLeft') next = keys[(index + keys.length - 1) % keys.length]
  if (event.key === 'Home') next = keys[0]
  if (event.key === 'End') next = keys[keys.length - 1]
  if (!next) return
  event.preventDefault()
  void selectTab(next)
  findTabButton(next)?.focus()
}

// 原生校验、业务校验和新手引导共用定位流程，先展示页签，再滚动及聚焦目标。
async function revealElement(element: HTMLElement, focus = true) {
  const key = element.closest<HTMLElement>('[data-settings-tab]')?.dataset.settingsTab
  if (!key || !visibleTabs.value.some(tab => tab.key === key)) return
  activeTab.value = key
  element.dispatchEvent(new Event('form-field-reveal', { bubbles: true }))
  await nextTick()
  element.scrollIntoView?.({ block: 'center', inline: 'nearest' })
  if (focus) {
    const selector = 'input, textarea, button, select, [tabindex]'
    const target = element.matches(selector) ? element
      : element.querySelector<HTMLElement>(selector) ?? element.parentElement?.querySelector<HTMLElement>(selector)
    target?.focus({ preventScroll: true })
  }
}

async function revealField(selector: string) {
  const element = rootRef.value?.querySelector<HTMLElement>(selector)
  if (element) await revealElement(element)
}

async function validate(): Promise<boolean> {
  const fields = rootRef.value?.querySelectorAll<HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement>(
    'input, textarea, select',
  )
  // 不依赖浏览器提交时自动聚焦，隐藏页签中的无效输入也能正常报告。
  const invalid = fields && Array.from(fields).find(field => field.willValidate && !field.validity.valid)
  if (!invalid) return true
  await revealElement(invalid)
  invalid.reportValidity()
  return false
}

function onTourReveal(event: Event) {
  if (event.target instanceof HTMLElement) void revealElement(event.target, false)
}

defineExpose({ validate, revealField, selectTab, activeTab })
</script>

<style scoped>
.settings-tabs {
  display: flex;
  height: min(68dvh, 760px);
  flex: 1 1 auto;
  min-height: 0;
  min-width: 0;
  flex-direction: column;
}

.settings-tab-list {
  @apply flex shrink-0 overflow-x-auto border-b border-gray-200 dark:border-dark-700;
}

.settings-tab {
  @apply shrink-0 whitespace-nowrap border-b-2 border-transparent px-4 py-3 text-sm font-medium text-gray-500 transition-colors hover:text-gray-900 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary-500 dark:text-gray-400 dark:hover:text-white;
}

.settings-tab-active {
  @apply border-primary-500 text-primary-700 dark:text-primary-300;
}

.settings-tab-content {
  @apply min-h-0 min-w-0 flex-1 overflow-y-auto overscroll-contain px-1 pb-2 pt-6;
}

/* 窄屏上限为 BREAKPOINT_SM（640px）减 1，与项目断点保持一致。 */
@media (max-width: 639px) {
  .settings-tab { @apply px-3; }
}
</style>
