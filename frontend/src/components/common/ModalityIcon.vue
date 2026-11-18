<template>
  <!-- 单个模态的图标块：10% 同色底 + 彩色图标，悬停和读屏给出模态名称。 -->
  <span
    class="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-compact"
    :class="toneClass"
    :title="label"
    role="img"
    :aria-label="label"
  >
    <Icon :name="iconName" size="xs" :stroke-width="2.5" />
  </span>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import type { IconName } from '@/components/icons/registry'
import type { ModelModality } from '@/utils/modelCapabilities'

const props = withDefaults(defineProps<{
  modality: ModelModality
  // inverse 用于始终深底的提示浮层，浅色主题下也使用深色配色。
  tone?: 'auto' | 'inverse'
}>(), {
  tone: 'auto',
})

const { t } = useI18n()

const ICONS: Record<ModelModality, IconName> = {
  text: 'modalityText',
  image: 'modalityImage',
  audio: 'modalityAudio',
  video: 'modalityVideo',
  pdf: 'modalityPdf',
}

// 配色参考 OpenRouter：文字=蓝、图片=绿、音频=琥珀、视频=玫红、PDF=紫。
const AUTO_CLASSES: Record<ModelModality, string> = {
  text: 'bg-blue-500/10 text-blue-600 dark:bg-blue-400/10 dark:text-blue-300',
  image: 'bg-green-500/10 text-green-600 dark:bg-green-400/10 dark:text-green-300',
  audio: 'bg-amber-500/10 text-amber-600 dark:bg-amber-400/10 dark:text-amber-300',
  video: 'bg-rose-500/10 text-rose-600 dark:bg-rose-400/10 dark:text-rose-300',
  pdf: 'bg-violet-500/10 text-violet-600 dark:bg-violet-400/10 dark:text-violet-300',
}

const INVERSE_CLASSES: Record<ModelModality, string> = {
  text: 'bg-blue-400/15 text-blue-300',
  image: 'bg-green-400/15 text-green-300',
  audio: 'bg-amber-400/15 text-amber-300',
  video: 'bg-rose-400/15 text-rose-300',
  pdf: 'bg-violet-400/15 text-violet-300',
}

const iconName = computed(() => ICONS[props.modality])
const toneClass = computed(() => (props.tone === 'inverse' ? INVERSE_CLASSES : AUTO_CLASSES)[props.modality])
const label = computed(() => t(`admin.modelAttributes.modalities.${props.modality}`))
</script>
