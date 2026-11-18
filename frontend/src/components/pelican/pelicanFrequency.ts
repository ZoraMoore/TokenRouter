// 预设和自定义频率共用计划的五字段 Cron，执行时间采用服务器时区。
export const pelicanFrequencies = {
  every15m: '*/15 * * * *',
  every30m: '*/30 * * * *',
  hourly: '0 * * * *',
  every2h: '0 */2 * * *',
  every6h: '0 */6 * * *',
  daily: '0 9 * * *',
} as const

export type PelicanFrequency = keyof typeof pelicanFrequencies | 'custom'

export function frequencyForCron(expression: string): PelicanFrequency {
  const normalized = expression.trim().replace(/\s+/g, ' ')
  return (Object.keys(pelicanFrequencies) as Array<keyof typeof pelicanFrequencies>)
    .find(key => pelicanFrequencies[key] === normalized) ?? 'custom'
}
