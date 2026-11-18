<template>
  <div class="relative min-w-0">
    <!-- 统计内容保留占位，成功提示退出后恢复显示。 -->
    <div :class="{ invisible: message !== null }" :aria-hidden="message !== null || undefined">
      <slot />
    </div>

    <div class="sr-only" role="status" aria-live="polite" aria-atomic="true">
      <span v-if="announcement" :key="sequence">{{ announcement }}</span>
    </div>

    <MotionTransition name="fade" @after-leave="finishFeedback">
      <div
        v-if="visible && message"
        :key="message.sequence"
        :data-sequence="message.sequence"
        data-testid="redeem-celebration"
        class="pointer-events-none absolute inset-0 flex min-w-0 items-center gap-3"
        aria-hidden="true"
      >
        <div
          v-if="confettiVisible"
          :data-sequence="message.sequence"
          class="redeem-confetti pointer-events-none absolute inset-x-0 -top-4 h-24 overflow-hidden"
          @animationend.self="finishConfetti"
        >
          <span
            v-for="particle in particles"
            :key="particle.id"
            class="redeem-confetti-piece absolute h-2 w-1"
            :class="particle.color"
            :style="particle.style"
          />
        </div>

        <div
          class="relative flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-emerald-100 text-emerald-600 dark:bg-emerald-900/30 dark:text-emerald-400"
          :data-sequence="message.sequence"
          :class="{ 'redeem-success-pop': iconAnimating }"
          @animationend.self="finishIcon"
        >
          <Icon name="check" size="lg" :animate-on-hover="false" />
        </div>
        <div class="relative min-w-0">
          <p class="truncate text-sm font-medium text-gray-600 dark:text-dark-300">{{ message.title }}</p>
          <p class="truncate text-lg font-semibold tabular-nums text-emerald-700 dark:text-emerald-400">
            {{ message.detail }}
          </p>
        </div>
      </div>
    </MotionTransition>
  </div>
</template>

<script setup lang="ts">
import { ref, watch } from 'vue'
import { usePreferredReducedMotion } from '@vueuse/core'
import Icon from '@/components/icons/Icon.vue'
import MotionTransition from '@/components/common/MotionTransition.vue'

const props = defineProps<{
  sequence: number
  title: string
  detail: string
}>()

const preference = usePreferredReducedMotion()
const visible = ref(false)
const confettiVisible = ref(false)
const iconAnimating = ref(false)
const announcement = ref('')
const message = ref<{ sequence: number; title: string; detail: string } | null>(null)

// 保留时长仅决定提示何时收起，CSS 动画通过完成事件自行清理。
const FEEDBACK_HOLD_MS = 3000
const colors = ['bg-primary-400', 'bg-emerald-400', 'bg-amber-400', 'bg-sky-400']
// 彩纸按固定轨迹在统计区附近散开。
const particles = Array.from({ length: 16 }, (_, id) => ({
  id,
  color: colors[id % colors.length],
  style: {
    left: `${8 + (id % 8) * 11}%`,
    '--confetti-x': `${(id % 2 === 0 ? -1 : 1) * (12 + (id % 4) * 8)}px`,
    '--confetti-rise': `${-12 - (id % 4) * 8}px`,
    '--confetti-fall': `${32 + (id % 3) * 12}px`,
    '--confetti-turn': `${(id % 2 === 0 ? -1 : 1) * (120 + id * 30)}deg`
  }
}))

// 序号为零表示取消；文案快照保留到退出完成，旧动画不能清除新结果。
watch(() => props.sequence, (sequence, _previous, onCleanup) => {
  visible.value = false
  confettiVisible.value = false
  iconAnimating.value = false
  announcement.value = ''
  if (sequence === 0) return

  message.value = { sequence, title: props.title, detail: props.detail }
  announcement.value = `${props.title} ${props.detail}`
  visible.value = true
  confettiVisible.value = preference.value !== 'reduce'
  iconAnimating.value = preference.value !== 'reduce'

  const timer = window.setTimeout(() => {
    visible.value = false
  }, FEEDBACK_HOLD_MS)
  onCleanup(() => window.clearTimeout(timer))
}, { immediate: true })

// 偏好变化立即取消装饰，恢复普通模式时也不重播本次庆祝。
watch(preference, (value) => {
  if (value !== 'reduce') return
  confettiVisible.value = false
  iconAnimating.value = false
})

function finishConfetti(event: AnimationEvent) {
  const element = event.currentTarget as HTMLElement
  if (Number(element.dataset.sequence) === message.value?.sequence) {
    confettiVisible.value = false
  }
}

function finishIcon(event: AnimationEvent) {
  const element = event.currentTarget as HTMLElement
  if (Number(element.dataset.sequence) === message.value?.sequence) {
    iconAnimating.value = false
  }
}

function finishFeedback(element: Element) {
  if (!visible.value && Number((element as HTMLElement).dataset.sequence) === message.value?.sequence) {
    message.value = null
  }
}
</script>

<style scoped>
.redeem-success-pop {
  animation: redeem-success-pop var(--motion-layout) var(--motion-ease);
}

.redeem-confetti {
  --celebration-duration: 2000ms;
  animation: redeem-confetti-fade var(--celebration-duration) linear;
}

.redeem-confetti-piece {
  top: 24px;
  animation: redeem-confetti-flight var(--celebration-duration) ease-out both;
}

@keyframes redeem-success-pop {
  0% {
    transform: scale(0.8);
  }
  65% {
    transform: scale(1.08);
  }
  100% {
    transform: scale(1);
  }
}

@keyframes redeem-confetti-fade {
  0%, 60% {
    opacity: 1;
  }
  100% {
    opacity: 0;
  }
}

@keyframes redeem-confetti-flight {
  0% {
    opacity: 0;
    transform: translateY(8px) rotate(0deg) scale(0.6);
  }
  12% {
    opacity: 0.9;
  }
  35% {
    transform: translate(var(--confetti-x), var(--confetti-rise)) rotate(var(--confetti-turn));
  }
  100% {
    transform: translate(var(--confetti-x), var(--confetti-fall)) rotate(var(--confetti-turn)) scale(0.8);
  }
}

@media (prefers-reduced-motion: reduce) {
  .redeem-success-pop,
  .redeem-confetti,
  .redeem-confetti-piece {
    animation: none;
  }

  .redeem-confetti {
    display: none;
  }
}
</style>
