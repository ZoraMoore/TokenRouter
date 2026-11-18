<template>
  <div class="usage-sparkline relative" aria-hidden="true">
    <!-- 数据变化时按路径重建，从左到右重新描一遍；裁剪放在外层，SVG 上的百分比会按 viewBox 换算 -->
    <div :key="path" class="sparkline-wipe h-full w-full">
      <svg class="h-full w-full overflow-visible" viewBox="0 0 100 32" preserveAspectRatio="none" focusable="false">
        <path
          v-if="path"
          :d="path"
          fill="none"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
          stroke-linejoin="round"
          vector-effect="non-scaling-stroke"
        />
      </svg>
    </div>
    <!-- 末端圆点用 HTML 绘制，SVG 被拉伸时圆点不会变成椭圆 -->
    <span
      v-if="endPoint"
      :key="`end-${path}`"
      data-testid="sparkline-end"
      class="sparkline-end absolute h-1.5 w-1.5 rounded-full bg-current"
      :style="{ left: `${endPoint.x}%`, top: `${endPoint.y}%` }"
    ></span>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  // 按时间排列的数值，null 表示该时段没有可计算的值，跳过后与前后时段相连，与主图一致。
  values: Array<number | null>
}>()

const WIDTH = 100
const HEIGHT = 32
// 上下预留线条绘制空间。
const PADDING = 2

// coordinates 以 0 为底线按最大值等比缩放，全为 0 时贴底；null 时段不出点。
const coordinates = computed(() => {
  const values = props.values
  const max = Math.max(0, ...values.filter((value): value is number => value !== null))
  const toY = (value: number) => {
    const ratio = max > 0 ? value / max : 0
    return HEIGHT - PADDING - ratio * (HEIGHT - PADDING * 2)
  }
  const step = values.length > 1 ? WIDTH / (values.length - 1) : 0
  return values
    .map((value, index) => (value === null ? null : { x: index * step, y: toY(value) }))
    .filter((point): point is { x: number; y: number } => point !== null)
})

const path = computed(() => {
  const points = coordinates.value
  if (points.length === 0) return ''
  // 只有一个时段时画成横线，否则只剩一个看不见的点。
  if (props.values.length === 1) {
    const y = points[0].y.toFixed(2)
    return `M0 ${y} L${WIDTH} ${y}`
  }
  return `M${points.map((point) => `${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(' L')}`
})

// endPoint 是最后一个有值时段的位置，换算成容器百分比。
const endPoint = computed(() => {
  const points = coordinates.value
  if (points.length === 0) return null
  const last = props.values.length === 1 ? { x: WIDTH, y: points[0].y } : points[points.length - 1]
  return { x: (last.x / WIDTH) * 100, y: (last.y / HEIGHT) * 100 }
})
</script>

<style scoped>
/* 描线用裁剪从左向右展开，non-scaling-stroke 下 stroke-dash 动画不可靠。上下留出余量，不裁掉线帽。 */
.sparkline-wipe {
  animation: sparkline-wipe var(--dash-sparkline-ms, 700ms) var(--motion-ease) both;
}

.sparkline-end {
  transform: translate(-50%, -50%);
  animation: sparkline-end var(--motion-normal) var(--motion-ease) var(--dash-sparkline-ms, 700ms) both;
}

@keyframes sparkline-wipe {
  from {
    clip-path: inset(-20% 100% -20% 0);
  }
  to {
    clip-path: inset(-20% 0 -20% 0);
  }
}

@keyframes sparkline-end {
  from {
    opacity: 0;
    transform: translate(-50%, -50%) scale(0);
  }
  to {
    opacity: 1;
    transform: translate(-50%, -50%) scale(1);
  }
}

@media (prefers-reduced-motion: reduce) {
  .sparkline-wipe,
  .sparkline-end {
    animation: none;
  }
}
</style>
