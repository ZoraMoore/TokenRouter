<template>
  <div v-if="contactEntries.length" class="relative" ref="dropdownRef">
    <button
      type="button"
      data-testid="contact-support-toggle"
      :class="showLabel ? 'header-status-labeled-button' : 'header-status-icon-button'"
      :aria-label="t('common.contactSupport')"
      :aria-expanded="isOpen"
      :title="t('common.contactSupport')"
      @click="toggleDropdown"
    >
      <Icon name="chat" size="md" />
      <span v-if="showLabel" class="max-w-14 truncate text-xs leading-none sm:max-w-24">{{ t('common.contactSupport') }}</span>
    </button>

    <!-- 联系方式逐条成行：链接在新窗口打开，纯文本点击复制。 -->
    <MotionTransition name="dropdown-fade">
      <div v-if="isOpen" class="dropdown fixed right-4 top-[var(--header-h)] z-50 mt-2 w-72 origin-top-right py-0 sm:absolute sm:right-0 sm:top-full">
        <div class="menu-section">
          <div class="menu-heading">{{ t('common.contactSupport') }}</div>
          <template v-for="(entry, idx) in contactEntries" :key="idx">
            <a
              v-if="entry.url"
              :href="entry.url"
              target="_blank"
              rel="noopener noreferrer"
              class="menu-item"
              :title="entry.value"
              @click="closeDropdown"
            >
              <span v-if="entry.label" class="shrink-0 font-normal text-primary-900/60 dark:text-dark-400">{{ entry.label }}</span>
              <span class="ml-auto min-w-0 truncate">{{ formatContactUrl(entry.url) }}</span>
              <Icon name="externalLink" size="sm" class="shrink-0" />
            </a>
            <button
              v-else
              type="button"
              class="menu-item"
              :title="t('common.copy')"
              @click="copyContact(entry.value)"
            >
              <span v-if="entry.label" class="shrink-0 font-normal text-primary-900/60 dark:text-dark-400">{{ entry.label }}</span>
              <span class="ml-auto min-w-0 truncate tabular-nums">{{ entry.value }}</span>
              <Icon name="copy" size="sm" class="shrink-0" />
            </button>
          </template>
        </div>
      </div>
    </MotionTransition>
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import MotionTransition from '@/components/common/MotionTransition.vue'
import Icon from '@/components/icons/Icon.vue'
import { useClipboard } from '@/composables/useClipboard'
import { useAppStore } from '@/stores'

interface ContactEntry {
  label: string
  value: string
  url: string
}

withDefaults(defineProps<{ showLabel?: boolean }>(), { showLabel: false })

const { t } = useI18n()
const appStore = useAppStore()
const { copyToClipboard } = useClipboard()

const isOpen = ref(false)
const dropdownRef = ref<HTMLElement | null>(null)

// 联系客服是自由文本（如“闲聊群(QQ)：123，TG群：https://t.me/xxx”），
// 按逗号、分号或换行拆条，再按冒号拆出“标签：值”，URL 渲染为可点击链接。
const contactEntries = computed<ContactEntry[]>(() => {
  const raw = appStore.contactInfo?.trim()
  if (!raw) return []
  return raw
    .split(/[，,;；\n]+/)
    .map(part => part.trim())
    .filter(Boolean)
    .map(part => {
      // 找第一个不属于协议（://）的冒号作为“标签：值”分隔符。
      let sep = -1
      for (let i = 0; i < part.length; i++) {
        const ch = part[i]
        if (ch === '：') { sep = i; break }
        if (ch === ':' && part.slice(i, i + 3) !== '://') { sep = i; break }
      }
      let label = ''
      let value = part
      if (sep > 0) {
        label = part.slice(0, sep).trim()
        value = part.slice(sep + 1).trim()
      }
      const url = /^https?:\/\/\S+$/.test(value) ? value : ''
      return { label, value, url }
    })
    .filter(e => e.value)
})

function toggleDropdown() {
  isOpen.value = !isOpen.value
}

function closeDropdown() {
  isOpen.value = false
}

// 面板宽度有限，链接只展示去掉协议和末尾斜杠后的地址。
function formatContactUrl(url: string) {
  return url.replace(/^https?:\/\//, '').replace(/\/$/, '')
}

async function copyContact(value: string) {
  await copyToClipboard(value)
  closeDropdown()
}

function handleClickOutside(event: MouseEvent) {
  if (dropdownRef.value && !dropdownRef.value.contains(event.target as Node)) {
    closeDropdown()
  }
}

onMounted(() => {
  document.addEventListener('click', handleClickOutside)
})

onBeforeUnmount(() => {
  document.removeEventListener('click', handleClickOutside)
})
</script>
