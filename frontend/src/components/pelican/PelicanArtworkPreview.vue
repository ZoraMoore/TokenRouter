<template>
  <span
    ref="stage" class="block w-full bg-white" data-testid="pelican-artwork-preview"
    :class="mode === 'actual' ? 'overflow-auto' : 'overflow-clip'"
    :tabindex="mode === 'actual' ? 0 : undefined"
    :role="mode === 'actual' ? 'region' : undefined"
    :aria-label="mode === 'actual' ? alt : undefined"
  >
    <span
      class="relative flex h-full min-h-full min-w-full items-center justify-center overflow-clip"
      :style="mode === 'actual' ? canvasStyle : undefined"
    >
      <iframe
        v-if="document" :srcdoc="document" :title="alt"
        sandbox="allow-scripts" referrerpolicy="no-referrer"
        class="absolute block max-w-none border-0"
        :style="{ ...canvasStyle, transform: 'scale(' + scale + ')' }"
      />
      <img
        v-else
        :src="source" :alt="alt" decoding="async" class="absolute block max-w-none"
        :style="{ ...canvasStyle, transform: 'scale(' + scale + ')' }"
        @error="emit('error')"
      />
    </span>
  </span>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'

const props = withDefaults(defineProps<{ source: string; alt: string; document?: string; mode?: 'fit' | 'actual' }>(), { mode: 'fit', document: '' })
const emit = defineEmits<{ error: [] }>()
const stage = ref<HTMLElement | null>(null)
const available = ref({ width: 0, height: 0 })
let observer: ResizeObserver | undefined

function validDimension(value: number): boolean {
  return Number.isFinite(value) && value > 0 && value <= 8192
}

function pixels(value: string | null): number {
  const match = /^\s*(\d*\.?\d+(?:e[+-]?\d+)?)(?:px)?\s*$/i.exec(value || '')
  return Number(match?.[1])
}

// 两种显示模式使用同一张 SVG 画布，图中的百分比和视口单位都按这张画布计算。
const canvas = computed(() => {
  const fallback = { width: 1024, height: 768 }
  const svg = decodeURIComponent(props.source.slice(props.source.indexOf(',') + 1))
  // 画布尺寸来自根标签。正文中的图形和动画交给 img 解码。
  const opening = /^<svg(?=[\s>])(?:[^"'>]|"[^"]*"|'[^']*')*>/i.exec(svg)?.[0]
  if (!opening) return fallback
  const root = new DOMParser().parseFromString(opening + '</svg>', 'image/svg+xml').documentElement
  let width = pixels(root.getAttribute('width'))
  let height = pixels(root.getAttribute('height'))
  const box = root.getAttribute('viewBox')?.trim().split(/[\s,]+/).map(Number)
  if (box?.length === 4 && box.every(Number.isFinite) && validDimension(box[2]) && validDimension(box[3])) {
    if (!validDimension(width) && !validDimension(height)) {
      width = box[2]
      height = box[3]
    } else if (!validDimension(width)) {
      width = height * box[2] / box[3]
    } else if (!validDimension(height)) {
      height = width * box[3] / box[2]
    }
  }
  return validDimension(width) && validDimension(height) ? { width, height } : fallback
})

const canvasStyle = computed(() => ({ width: canvas.value.width + 'px', height: canvas.value.height + 'px' }))
const scale = computed(() => props.mode === 'actual' ? 1
  : Math.min(1, available.value.width / canvas.value.width, available.value.height / canvas.value.height))

function measure() {
  const element = stage.value
  if (element) available.value = { width: element.clientWidth, height: element.clientHeight }
}

onMounted(() => {
  measure()
  if (typeof ResizeObserver === 'undefined') {
    window.addEventListener('resize', measure)
  } else if (stage.value) {
    observer = new ResizeObserver(measure)
    observer.observe(stage.value)
  }
})
onBeforeUnmount(() => {
  observer?.disconnect()
  window.removeEventListener('resize', measure)
})

watch(() => props.mode, () => {
  if (stage.value) {
    stage.value.scrollLeft = 0
    stage.value.scrollTop = 0
  }
}, { flush: 'post' })
</script>
