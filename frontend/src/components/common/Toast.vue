<template>
  <Teleport to="body">
    <!-- 窄屏横向铺满，桌面端固定在右上角。 -->
    <div
      class="pointer-events-none fixed inset-x-4 top-4 z-toast flex flex-col items-stretch gap-2 sm:left-auto sm:w-[360px]"
      aria-live="polite"
      aria-atomic="true"
    >
      <TransitionGroup name="motion-list" @before-leave="prepareListLeave" @before-enter="restoreEnteringElement">
        <div
          v-for="toast in toasts"
          :key="toast.id"
          :role="toast.type === 'error' ? 'alert' : 'status'"
          data-testid="toast"
          :data-toast-type="toast.type"
          class="pointer-events-auto flex w-full items-start gap-3 rounded-surface border border-gray-200 bg-white py-3 pl-4 pr-3 shadow-lg shadow-black/[0.06] dark:border-dark-600 dark:bg-dark-900 dark:shadow-black/40"
        >
          <!-- 状态只由图标颜色区分，卡片保持中性。 -->
          <Icon
            :name="getToastIconName(toast.type)"
            size="sm"
            :stroke-width="2"
            :animate-on-hover="false"
            :class="['mt-0.5 shrink-0', getIconColor(toast.type)]"
            aria-hidden="true"
          />

          <div class="min-w-0 flex-1">
            <p v-if="toast.title" class="text-sm font-medium leading-5 text-gray-900 dark:text-dark-50">
              {{ toast.title }}
            </p>
            <p
              :class="[
                'break-words text-sm leading-5',
                toast.title
                  ? 'mt-0.5 text-gray-500 dark:text-dark-300'
                  : 'text-gray-900 dark:text-dark-100'
              ]"
            >
              {{ toast.message }}
            </p>
          </div>

          <button
            type="button"
            class="-my-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-compact text-gray-400 transition-colors hover:bg-gray-100 hover:text-gray-700 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-black/10 dark:text-dark-400 dark:hover:bg-dark-800 dark:hover:text-dark-100 dark:focus-visible:ring-primary-500/50"
            :aria-label="t('common.close')"
            @click="removeToast(toast.id)"
          >
            <Icon name="x" size="sm" :stroke-width="1.75" :animate-on-hover="false" />
          </button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<script setup lang="ts">
import { prepareListLeave, restoreEnteringElement } from '@/utils/leavingElement'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import Icon from '@/components/icons/Icon.vue'
import { useAppStore } from '@/stores/app'

const { t } = useI18n()
const appStore = useAppStore()

const toasts = computed(() => appStore.toasts)

const getToastIconName = (type: string): 'checkCircle' | 'xCircle' | 'exclamationTriangle' | 'infoCircle' => {
  switch (type) {
    case 'success':
      return 'checkCircle'
    case 'error':
      return 'xCircle'
    case 'warning':
      return 'exclamationTriangle'
    case 'info':
    default:
      return 'infoCircle'
  }
}

// 深色模式提亮一档，保证图标在近黑底上仍然清楚。
const getIconColor = (type: string): string => {
  const colors: Record<string, string> = {
    success: 'text-emerald-500 dark:text-emerald-400',
    error: 'text-red-500 dark:text-red-400',
    warning: 'text-amber-500 dark:text-amber-400',
    info: 'text-blue-500 dark:text-blue-400'
  }
  return colors[type] || colors.info
}

const removeToast = (id: string) => {
  appStore.hideToast(id)
}
</script>
