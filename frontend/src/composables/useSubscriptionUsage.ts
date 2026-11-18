import { useI18n } from 'vue-i18n'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import type { UserSubscription } from '@/types'
import { formatDateTimeToMinute } from '@/utils/format'

// SubscriptionUsageWindow 描述订阅在某个周期内的已用额度与上限。
export interface SubscriptionUsageWindow {
  key: 'daily' | 'weekly' | 'monthly'
  label: string
  used: number
  limit: number | null
}

// useSubscriptionUsage 汇总订阅用量的计算与展示规则，供顶栏订阅弹层和兑换页共用。
export function useSubscriptionUsage() {
  const { t } = useI18n()
  const { formatBalanceAmount } = useBalanceDisplay()

  // usageWindows 只返回配置了正数上限的周期，没有任何上限的订阅视为无限制。
  function usageWindows(subscription: UserSubscription): SubscriptionUsageWindow[] {
    const windows: SubscriptionUsageWindow[] = [
      {
        key: 'daily',
        label: t('subscriptionProgress.daily'),
        used: subscription.daily_usage_usd || 0,
        limit: subscription.daily_limit_usd
      },
      {
        key: 'weekly',
        label: t('subscriptionProgress.weekly'),
        used: subscription.weekly_usage_usd || 0,
        limit: subscription.weekly_limit_usd
      },
      {
        key: 'monthly',
        label: t('subscriptionProgress.monthly'),
        used: subscription.monthly_usage_usd || 0,
        limit: subscription.monthly_limit_usd
      }
    ]
    return windows.filter((window) => window.limit != null && window.limit > 0)
  }

  function getMaxUsagePercentage(subscription: UserSubscription): number {
    const windows = usageWindows(subscription)
    if (windows.length === 0) return 0
    return Math.max(...windows.map((window) => ((window.used || 0) / (window.limit || 1)) * 100))
  }

  function isUnlimited(subscription: UserSubscription): boolean {
    return usageWindows(subscription).length === 0
  }

  // sortByUsage 按最高周期用量降序排列，让最接近上限的订阅排在最前。
  function sortByUsage(subscriptions: UserSubscription[]): UserSubscription[] {
    return [...subscriptions].sort((a, b) => getMaxUsagePercentage(b) - getMaxUsagePercentage(a))
  }

  function getProgressDotClass(subscription: UserSubscription): string {
    if (isUnlimited(subscription)) return 'bg-emerald-500'
    const percentage = getMaxUsagePercentage(subscription)
    if (percentage >= 90) return 'bg-red-500'
    if (percentage >= 70) return 'bg-orange-500'
    return 'bg-green-500'
  }

  function getProgressBarClass(used: number, limit: number | null): string {
    if (!limit || limit === 0) return 'bg-gray-400'
    const percentage = (used / limit) * 100
    if (percentage >= 90) return 'bg-red-500'
    if (percentage >= 70) return 'bg-orange-500'
    return 'bg-green-500'
  }

  function getProgressWidth(used: number, limit: number | null): string {
    if (!limit || limit === 0) return '0%'
    return `${Math.min((used / limit) * 100, 100)}%`
  }

  function formatUsage(used: number, limit: number | null): string {
    const usedValue = formatBalanceAmount(used, { fractionDigits: 2 })
    const limitValue = limit == null ? '∞' : formatBalanceAmount(limit, { fractionDigits: 2 })
    return `${usedValue}/${limitValue}`
  }

  function formatExpiration(expiresAt: string): string {
    const time = formatDateTimeToMinute(expiresAt)
    return time ? t('subscriptionProgress.expiresAt', { time }) : ''
  }

  // getDaysRemainingClass 在到期前一周转为橙色、前三天转为红色。
  function getDaysRemainingClass(expiresAt: string): string {
    const days = Math.ceil((new Date(expiresAt).getTime() - Date.now()) / (1000 * 60 * 60 * 24))
    if (days <= 3) return 'text-red-600 dark:text-red-400'
    if (days <= 7) return 'text-orange-600 dark:text-orange-400'
    return 'text-gray-500 dark:text-dark-400'
  }

  return {
    usageWindows,
    isUnlimited,
    sortByUsage,
    getProgressDotClass,
    getProgressBarClass,
    getProgressWidth,
    formatUsage,
    formatExpiration,
    getDaysRemainingClass
  }
}
