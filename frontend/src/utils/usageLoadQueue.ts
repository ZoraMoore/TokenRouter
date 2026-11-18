/**
 * Usage request scheduler - throttles Anthropic API calls by proxy exit.
 *
 * Anthropic OAuth/setup-token providers sharing the same proxy exit are placed
 * into a serial queue with a random 1-2s delay between requests, preventing
 * upstream 429 rate-limit errors.
 *
 * Proxy identity = host:port:username - two proxy records pointing to the
 * same exit share a single queue. Providers without a proxy go into a
 * "direct" queue.
 *
 * All other platforms bypass the queue and execute immediately.
 */

import type { Provider } from '@/types'

const GROUP_DELAY_MIN_MS = 1000
const GROUP_DELAY_MAX_MS = 2000

type Task<T> = {
  fn: () => Promise<T>
  resolve: (value: T) => void
  reject: (reason: unknown) => void
}

const queues = new Map<string, Task<unknown>[]>()
const running = new Set<string>()

/** Whether this provider needs throttled queuing. */
function needsThrottle(provider: Provider): boolean {
  return (
    provider.platform === 'anthropic' &&
    (provider.type === 'oauth' || provider.type === 'setup-token')
  )
}

/** Build a queue key from proxy connection details. */
function buildGroupKey(provider: Provider): string {
  const proxy = provider.proxy
  const proxyIdentity = proxy
    ? `${proxy.host}:${proxy.port}:${proxy.username || ''}`
    : 'direct'
  return `anthropic:${proxyIdentity}`
}

async function drain(groupKey: string) {
  if (running.has(groupKey)) return
  running.add(groupKey)

  const queue = queues.get(groupKey)
  while (queue && queue.length > 0) {
    const task = queue.shift()!
    try {
      const result = await task.fn()
      task.resolve(result)
    } catch (err) {
      task.reject(err)
    }
    if (queue.length > 0) {
      const jitter = GROUP_DELAY_MIN_MS + Math.random() * (GROUP_DELAY_MAX_MS - GROUP_DELAY_MIN_MS)
      await new Promise((r) => setTimeout(r, jitter))
    }
  }

  running.delete(groupKey)
  queues.delete(groupKey)
}

/**
 * Schedule a usage fetch. Anthropic providers are queued by proxy exit;
 * all other platforms execute immediately.
 */
export function enqueueUsageRequest<T>(
  provider: Provider,
  fn: () => Promise<T>
): Promise<T> {
  if (!needsThrottle(provider)) {
    return fn()
  }

  const key = buildGroupKey(provider)

  return new Promise<T>((resolve, reject) => {
    let queue = queues.get(key)
    if (!queue) {
      queue = []
      queues.set(key, queue)
    }
    queue.push({ fn, resolve, reject } as Task<unknown>)
    drain(key)
  })
}
