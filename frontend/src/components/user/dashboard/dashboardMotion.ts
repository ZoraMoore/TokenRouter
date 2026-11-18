import type { CSSProperties } from 'vue'

// 仪表盘专用动效的时长都在这里定义，脚本直接引用常量，样式通过 dashboardMotionVars 注入的 CSS 变量读取。

// COUNT_UP_MS 是指标数字从旧值滚动到新值的时长。
export const COUNT_UP_MS = 900

// CHART_REVEAL_MS 是趋势图重新取数后从左到右描线的总时长。
export const CHART_REVEAL_MS = 800

// SPARKLINE_DRAW_MS 是指标卡迷你走势线的描线时长。
export const SPARKLINE_DRAW_MS = 700

// DASH_RISE_MS 是页面各区块首次进入时上移淡入的时长，相邻区块错开 DASH_RISE_STEP_MS。
export const DASH_RISE_MS = 420
export const DASH_RISE_STEP_MS = 60

// HEATMAP_CELL_ENTER_MS 是热力图单个格子的入场时长；相邻两列错开 HEATMAP_WAVE_STEP_MS，最晚一列不超过 HEATMAP_WAVE_MAX_MS。
export const HEATMAP_CELL_ENTER_MS = 320
export const HEATMAP_WAVE_STEP_MS = 8
export const HEATMAP_WAVE_MAX_MS = 480

// TOP_MODEL_BAR_MS 是模型占比条伸展的时长，相邻两行错开 TOP_MODEL_BAR_STEP_MS。
export const TOP_MODEL_BAR_MS = 600
export const TOP_MODEL_BAR_STEP_MS = 60

// dashboardMotionVars 把时长常量转成 CSS 变量，挂在页面根节点上供各组件样式引用。
export const dashboardMotionVars: CSSProperties = {
  '--dash-rise-ms': `${DASH_RISE_MS}ms`,
  '--dash-rise-step-ms': `${DASH_RISE_STEP_MS}ms`,
  '--dash-sparkline-ms': `${SPARKLINE_DRAW_MS}ms`,
  '--dash-heatmap-enter-ms': `${HEATMAP_CELL_ENTER_MS}ms`,
  '--dash-top-model-bar-ms': `${TOP_MODEL_BAR_MS}ms`,
} as CSSProperties
