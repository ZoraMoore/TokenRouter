<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { useNavigationLoadingState } from '@/composables/useNavigationLoading'

const { t } = useI18n()
const { isLoading, isNavigating, navigationId, finishNavigation } = useNavigationLoadingState()

// CSS 动画结束事件触发隐藏。
const onAnimationEnd = (event: AnimationEvent) => {
  if (!event.animationName.startsWith('navigation-complete')) return
  const target = event.currentTarget as HTMLElement
  finishNavigation(Number(target.dataset.navigationId))
}
</script>

<template>
  <div
    v-if="isLoading"
    :key="navigationId"
    class="navigation-progress"
    :class="{ 'is-complete': !isNavigating }"
    role="progressbar"
    :aria-label="t('common.loading')"
    aria-valuemin="0"
    aria-valuemax="100"
  >
    <!-- 进度线表示导航仍在进行。 -->
    <div
      class="navigation-progress-bar"
      :data-navigation-id="navigationId"
      @animationend="onAnimationEnd"
    />
  </div>
</template>

<style scoped>
.navigation-progress {
  position: fixed;
  inset: 0 0 auto;
  height: 2px;
  z-index: var(--z-toast);
  overflow: hidden;
  pointer-events: none;
}

.navigation-progress-bar {
  height: 100%;
  background: theme('colors.primary.500');
  transform-origin: left;
  animation: navigation-advance 8s cubic-bezier(0.1, 0.5, 0.2, 1) forwards;
}

.navigation-progress.is-complete .navigation-progress-bar {
  transform: scaleX(1);
  animation: navigation-complete 180ms ease-out forwards;
}

@keyframes navigation-advance {
  from { transform: scaleX(0.08); }
  to { transform: scaleX(0.9); }
}

@keyframes navigation-complete {
  from { opacity: 1; }
  to { opacity: 0; }
}

@media (prefers-reduced-motion: reduce) {
  .navigation-progress-bar {
    animation: none;
    transform: scaleX(0.35);
  }

  .navigation-progress.is-complete .navigation-progress-bar {
    animation-duration: 1ms;
  }
}
</style>
