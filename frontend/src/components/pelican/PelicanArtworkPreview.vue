<template>
  <span
    ref="stage" class="block w-full bg-white" data-testid="pelican-artwork-preview"
    :class="mode === 'actual' ? 'overflow-auto' : 'overflow-clip'"
    :tabindex="mode === 'actual' ? 0 : undefined"
    :role="mode === 'actual' ? 'region' : undefined"
    :aria-label="mode === 'actual' ? alt : undefined"
  >
    <span class="flex min-h-full min-w-full items-center justify-center" :style="surfaceStyle">
      <span class="relative block shrink-0 overflow-clip" :style="artworkStyle">
        <iframe
          v-if="frame" ref="artworkFrame" :srcdoc="frame.srcdoc" :title="alt"
          :sandbox="interactive ? 'allow-scripts allow-pointer-lock' : 'allow-scripts'" referrerpolicy="no-referrer"
          scrolling="no" :tabindex="interactive ? undefined : -1" :class="{ 'pointer-events-none': !interactive }"
          class="absolute left-0 top-0 block max-w-none origin-top-left border-0"
          :style="renderStyle"
        />
        <img
          v-else-if="artwork.image"
          :src="artwork.image" :alt="alt" decoding="async"
          class="absolute left-0 top-0 block max-w-none origin-top-left"
          :style="renderStyle" @error="emit('error')"
        />
      </span>
    </span>
  </span>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { PelicanArtworkBody } from './pelicanArtwork'
import { createPelicanFrame, fitPelicanCanvas, pelicanPreviewUpdateLimit, pelicanSvgCanvas, readPelicanCanvas, type PelicanCanvas, type PelicanPreviewMode } from './pelicanFrame'

const props = withDefaults(defineProps<{ artwork: PelicanArtworkBody; alt: string; mode?: PelicanPreviewMode; interactive?: boolean }>(), { mode: 'fit' })
const emit = defineEmits<{ error: [] }>()
const stage = ref<HTMLElement | null>(null)
const artworkFrame = ref<HTMLIFrameElement | null>(null)
const available = ref({ width: 0, height: 0 })
const measured = ref<PelicanCanvas | null>(null)
let observer: ResizeObserver | undefined
let measurementCount = 0

const frame = computed(() => props.artwork.html ? createPelicanFrame(props.artwork.html, props.interactive) : null)
const viewport = computed(() => {
  if (frame.value) return frame.value.viewport
  const image = props.artwork.image || ''
  const markup = decodeURIComponent(image.slice(image.indexOf(',') + 1))
  const opening = /^<svg(?=[\s>])(?:[^"'>]|"[^"]*"|'[^']*')*>/i.exec(markup)?.[0]
  const svg = opening ? new DOMParser().parseFromString(opening + '</svg>', 'image/svg+xml').documentElement : null
  return pelicanSvgCanvas(svg)
})
const canvas = computed(() => measured.value ?? viewport.value)
const scale = computed(() => props.mode === 'actual' ? 1 : fitPelicanCanvas(canvas.value, available.value))
const artworkStyle = computed(() => pixelSize(canvas.value.width * scale.value, canvas.value.height * scale.value))
const surfaceStyle = computed(() => pixelSize(
  Math.max(canvas.value.width * scale.value, available.value.width),
  Math.max(canvas.value.height * scale.value, available.value.height),
))
const renderStyle = computed(() => ({
  ...pixelSize(viewport.value.width, viewport.value.height),
  // 作品框架在固定视口内缩小溢出内容。外层恢复该比例，再按容器缩放。
  transform: 'scale(' + scale.value / fitPelicanCanvas(canvas.value, viewport.value) + ')',
}))

function pixelSize(width: number, height: number) {
  return { width: width + 'px', height: height + 'px' }
}

function measureStage() {
  const element = stage.value
  if (element) available.value = { width: element.clientWidth, height: element.clientHeight }
}

function receiveCanvas(event: MessageEvent) {
  if (!frame.value || measurementCount >= pelicanPreviewUpdateLimit) return
  const size = readPelicanCanvas(event, artworkFrame.value?.contentWindow ?? undefined, frame.value.channel)
  if (!size || size.width < canvas.value.width || size.height < canvas.value.height) return
  if (size.width === canvas.value.width && size.height === canvas.value.height) return
  measured.value = size
  measurementCount++
}

watch(frame, () => {
  measured.value = null
  measurementCount = 0
}, { flush: 'sync' })

onMounted(() => {
  measureStage()
  window.addEventListener('resize', measureStage)
  if (typeof ResizeObserver !== 'undefined' && stage.value) {
    observer = new ResizeObserver(measureStage)
    observer.observe(stage.value)
  }
  window.addEventListener('message', receiveCanvas)
})

onBeforeUnmount(() => {
  observer?.disconnect()
  window.removeEventListener('resize', measureStage)
  window.removeEventListener('message', receiveCanvas)
})

watch(() => props.mode, () => {
  if (stage.value) {
    stage.value.scrollLeft = 0
    stage.value.scrollTop = 0
  }
}, { flush: 'post' })
</script>
