import type { PelicanModel } from '@/api/pelicanTests'

export { useKeyedResource as usePelicanResource } from '@/composables/useKeyedResource'

export const PELICAN_SLOW_POLL_MS = 30_000
type PollablePlan = Pick<PelicanModel, 'run_state' | 'enabled'> & Partial<Pick<PelicanModel, 'latest' | 'artworks'>>

// 等待定时采样或人工复核时每 30 秒刷新，正在排队或作答时缩短间隔。
export function pelicanPollDelay(plans: readonly PollablePlan[]): number | undefined {
  if (plans.some(plan => plan.run_state === 'queued' || plan.run_state === 'running')) return 2500
  if (plans.some(plan => plan.enabled || plan.latest?.status === 'pending_review' ||
    plan.artworks?.some(art => art.status === 'pending_review'))) return PELICAN_SLOW_POLL_MS
}
