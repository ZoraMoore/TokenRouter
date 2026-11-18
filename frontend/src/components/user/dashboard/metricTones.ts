// 仪表盘各类指标图标的配色，指标卡和标题下的实时状态条共用，同一指标在两处颜色一致。
// 类名写成完整字符串，保证 Tailwind 能扫描到。
export const METRIC_TONES = {
  // 请求数与 RPM：品牌青，与请求数折线同色
  requests: { icon: 'text-primary-600 dark:text-primary-500', tile: 'bg-primary-500/10' },
  // Token 与 TPM
  tokens: { icon: 'text-blue-500 dark:text-blue-400', tile: 'bg-blue-500/10' },
  // 消费与今日消费
  cost: { icon: 'text-amber-500 dark:text-amber-400', tile: 'bg-amber-500/10' },
  // 缓存命中率：紫色，与命中率折线同色
  cacheHitRate: { icon: 'text-violet-500 dark:text-violet-400', tile: 'bg-violet-500/10' },
  // 平均耗时
  latency: { icon: 'text-rose-500 dark:text-rose-400', tile: 'bg-rose-500/10' },
} as const

export type MetricTone = keyof typeof METRIC_TONES
