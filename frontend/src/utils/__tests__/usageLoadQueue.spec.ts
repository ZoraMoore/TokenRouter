import { describe, expect, it } from 'vitest'
import { enqueueUsageRequest } from '../usageLoadQueue'
import type { Provider } from '@/types'

/** Helper to create a minimal Provider with proxy info */
function makeProvider(
  platform: string,
  type: string = 'oauth',
  proxy?: { host: string; port: number; username?: string | null } | null
): Provider {
  return {
    id: Math.floor(Math.random() * 10000),
    platform,
    type,
    name: 'test',
    status: 'active',
    proxy_id: proxy ? 1 : null,
    proxy: proxy
      ? { id: 1, name: 'p', protocol: 'http', host: proxy.host, port: proxy.port, username: proxy.username ?? null, status: 'active', created_at: '', updated_at: '' }
      : undefined,
    credentials: {},
    created_at: '',
    updated_at: ''
  } as unknown as Provider
}

describe('usageLoadQueue', () => {
  // ─── Anthropic 提供商：按代理出口排队 ───

  it('Anthropic 同代理出口串行执行，间隔 >= 1s', async () => {
    const timestamps: number[] = []
    const makeFn = () => async () => {
      timestamps.push(Date.now())
      return 'ok'
    }

    const acc = makeProvider('anthropic', 'oauth', { host: '1.2.3.4', port: 8080, username: 'u1' })

    const p1 = enqueueUsageRequest(acc, makeFn())
    const p2 = enqueueUsageRequest(acc, makeFn())
    const p3 = enqueueUsageRequest(acc, makeFn())

    await Promise.all([p1, p2, p3])

    expect(timestamps).toHaveLength(3)
    expect(timestamps[1] - timestamps[0]).toBeGreaterThanOrEqual(950)
    expect(timestamps[1] - timestamps[0]).toBeLessThan(2100)
    expect(timestamps[2] - timestamps[1]).toBeGreaterThanOrEqual(950)
    expect(timestamps[2] - timestamps[1]).toBeLessThan(2100)
  })

  it('Anthropic 不同代理出口并行执行', async () => {
    const timestamps: Record<string, number> = {}
    const makeTracked = (key: string) => async () => {
      timestamps[key] = Date.now()
      return key
    }

    const acc1 = makeProvider('anthropic', 'oauth', { host: '1.2.3.4', port: 8080, username: 'u1' })
    const acc2 = makeProvider('anthropic', 'oauth', { host: '5.6.7.8', port: 3128, username: 'u2' })

    const p1 = enqueueUsageRequest(acc1, makeTracked('proxy1'))
    const p2 = enqueueUsageRequest(acc2, makeTracked('proxy2'))

    await Promise.all([p1, p2])

    const spread = Math.abs(timestamps['proxy1'] - timestamps['proxy2'])
    expect(spread).toBeLessThan(50)
  })

  it('Anthropic 相同代理连接信息的不同提供商归为同一队列', async () => {
    const timestamps: number[] = []
    const makeFn = () => async () => {
      timestamps.push(Date.now())
      return 'ok'
    }

    const acc1 = makeProvider('anthropic', 'oauth', { host: '10.0.0.1', port: 3128, username: 'admin' })
    const acc2 = makeProvider('anthropic', 'setup-token', { host: '10.0.0.1', port: 3128, username: 'admin' })

    const p1 = enqueueUsageRequest(acc1, makeFn())
    const p2 = enqueueUsageRequest(acc2, makeFn())

    await Promise.all([p1, p2])

    expect(timestamps).toHaveLength(2)
    expect(timestamps[1] - timestamps[0]).toBeGreaterThanOrEqual(950)
  })

  it('Anthropic 直连（无代理）的提供商归为同一队列', async () => {
    const order: number[] = []
    const makeFn = (n: number) => async () => {
      order.push(n)
      return n
    }

    const acc1 = makeProvider('anthropic', 'oauth')
    const acc2 = makeProvider('anthropic', 'setup-token')

    const p1 = enqueueUsageRequest(acc1, makeFn(1))
    const p2 = enqueueUsageRequest(acc2, makeFn(2))

    await Promise.all([p1, p2])

    expect(order).toEqual([1, 2])
  })

  it('Anthropic 请求失败时 reject，后续任务继续执行', async () => {
    const results: string[] = []
    const acc = makeProvider('anthropic', 'oauth', { host: '99.99.99.99', port: 1234 })

    const p1 = enqueueUsageRequest(acc, async () => {
      throw new Error('fail')
    })
    const p2 = enqueueUsageRequest(acc, async () => {
      results.push('second')
      return 'ok'
    })

    await expect(p1).rejects.toThrow('fail')
    await p2
    expect(results).toEqual(['second'])
  })

  // ─── 非 Anthropic 平台：直接执行，不排队 ───

  it('非 Anthropic 平台直接执行，不排队', async () => {
    const timestamps: number[] = []
    const makeFn = () => async () => {
      timestamps.push(Date.now())
      return 'ok'
    }

    // 同一代理的 Gemini 提供商 — 应当并行，不排队
    const acc1 = makeProvider('gemini', 'oauth', { host: '1.2.3.4', port: 8080 })
    const acc2 = makeProvider('gemini', 'oauth', { host: '1.2.3.4', port: 8080 })

    const p1 = enqueueUsageRequest(acc1, makeFn())
    const p2 = enqueueUsageRequest(acc2, makeFn())

    await Promise.all([p1, p2])

    expect(timestamps).toHaveLength(2)
    // 并行执行，几乎同时完成
    expect(Math.abs(timestamps[1] - timestamps[0])).toBeLessThan(50)
  })

  it('OpenAI 平台直接执行，不排队', async () => {
    const timestamps: number[] = []
    const makeFn = () => async () => {
      timestamps.push(Date.now())
      return 'ok'
    }

    const acc1 = makeProvider('openai', 'oauth', { host: '1.2.3.4', port: 8080 })
    const acc2 = makeProvider('openai', 'oauth', { host: '1.2.3.4', port: 8080 })

    const p1 = enqueueUsageRequest(acc1, makeFn())
    const p2 = enqueueUsageRequest(acc2, makeFn())

    await Promise.all([p1, p2])

    expect(timestamps).toHaveLength(2)
    expect(Math.abs(timestamps[1] - timestamps[0])).toBeLessThan(50)
  })

  // ─── Anthropic apikey 类型不排队 ───

  it('Anthropic apikey 类型直接执行，不排队', async () => {
    const timestamps: number[] = []
    const makeFn = () => async () => {
      timestamps.push(Date.now())
      return 'ok'
    }

    const acc1 = makeProvider('anthropic', 'apikey', { host: '1.2.3.4', port: 8080 })
    const acc2 = makeProvider('anthropic', 'apikey', { host: '1.2.3.4', port: 8080 })

    const p1 = enqueueUsageRequest(acc1, makeFn())
    const p2 = enqueueUsageRequest(acc2, makeFn())

    await Promise.all([p1, p2])

    expect(timestamps).toHaveLength(2)
    expect(Math.abs(timestamps[1] - timestamps[0])).toBeLessThan(50)
  })

  // ─── 返回值透传 ───

  it('返回值正确透传', async () => {
    const acc = makeProvider('anthropic', 'oauth')
    const result = await enqueueUsageRequest(acc, async () => {
      return { usage: 42 }
    })
    expect(result).toEqual({ usage: 42 })
  })

  it('非 Anthropic 返回值正确透传', async () => {
    const acc = makeProvider('gemini', 'oauth')
    const result = await enqueueUsageRequest(acc, async () => {
      return { quota: 100 }
    })
    expect(result).toEqual({ quota: 100 })
  })
})
