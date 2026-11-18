<template>
  <MotionTransition name="pop-float">
    <!-- 外层占满宽度只负责居中，点击穿透到下面的内容。 -->
    <div
      v-if="visible"
      class="pointer-events-none sticky bottom-6 flex justify-center"
      data-testid="settings-save-bar"
    >
      <!-- 外框 16px 圆角、8px 内边距，内部 8px 圆角的按钮与外框弧度同心。 -->
      <div
        class="pointer-events-auto flex max-w-full flex-wrap items-center gap-x-6 gap-y-2 rounded-dialog bg-gray-900 p-2 pl-5 text-white shadow-2xl ring-1 ring-black/10 dark:bg-dark-900 dark:text-dark-50 dark:ring-dark-500"
        role="status"
      >
        <p class="text-sm font-medium">{{ message }}</p>
        <div class="flex items-center gap-2">
          <slot />
        </div>
      </div>
    </div>
  </MotionTransition>
</template>

<script setup lang="ts">
import MotionTransition from '@/components/common/MotionTransition.vue'

// 整页设置有未保存的修改时，在视口底部居中浮出深色操作条，放弃和保存按钮由调用方放进默认插槽。
defineProps<{
  visible: boolean
  message: string
}>()
</script>
