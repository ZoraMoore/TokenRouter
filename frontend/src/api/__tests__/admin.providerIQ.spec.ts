import { afterEach, describe, expect, it, vi } from 'vitest'
import { ADMIN_UI_REQUEST_HEADER } from '@/api/adminUIRequest'
import { apiClient } from '@/api/client'
import { runIQQuality, runIQQuestion, runIQStateProbe, runIQTrace } from '@/api/admin/providerIQ'

const bank = { sha256: 'example', algorithm: 'modeltrace', models: ['a', 'b'] }
const sample = { challenge: { id: 'q1', prompt: '挑选数字', expected_count: 50 }, accepted: true, parsed_numbers: 50, minimum_numbers: 40, mapped_model: 'model-a', text: '5 7' }
const result = { requested_model: 'selected', reasoning_effort: 'default', bank, samples: [sample], status: 'complete', disagreement: false, created_at: '2026-11-19T04:00:00Z' }
const chunk = (type: string, data: unknown) => 'data: ' + JSON.stringify({ type, data }) + '\r\n\r\n'

function mockStream(input: string) {
  const bytes = new TextEncoder().encode(input)
  const body = new ReadableStream<Uint8Array>({ start(controller) {
    for (let i = 0; i < bytes.length; i += 7) controller.enqueue(bytes.slice(i, i + 7))
    controller.close()
  } })
  const fetch = vi.fn().mockResolvedValue(new Response(body, { headers: { 'content-type': 'text/event-stream' } }))
  vi.stubGlobal('fetch', fetch)
  return fetch
}

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks(); localStorage.clear() })

describe('账号答题接口', () => {
  it('题目和质量检测的 JSON 请求固定提供商 ID，异常响应无法作为答案', async () => {
    const signal = new AbortController().signal
    const input = { model_id: 'selected', reasoning_effort: '', question_kind: 'candy' as const, prompt: '数糖果', round_id: 'round-1', expected_answer: '21' }
    const sample = { question_kind: 'candy', requested_model: input.model_id, prompt: input.prompt, started_at: '2026-11-19T04:00:00Z', status: 'passed', latency_ms: 30, answer: '21' }
    const post = vi.spyOn(apiClient, 'post').mockResolvedValueOnce({ data: sample }).mockResolvedValueOnce({ data: { status: 'ok' } })
    await expect(runIQQuestion(22, input, signal)).resolves.toEqual(sample)
    expect(post).toHaveBeenNthCalledWith(1, '/admin/providers/22/iq-test', input, { signal, timeout: 0 })
    const model = { model_id: 'selected', reasoning_effort: 'high' }
    await expect(runIQQuality(30, model, signal)).rejects.toThrow('IQ_INVALID_RESULT')
    expect(post).toHaveBeenNthCalledWith(2, '/admin/providers/30/iq-test/quality', model, { signal, timeout: 0 })
  })
})

describe('账号状态探针接口', () => {
  const observed = {
    account_id: 22, requested_model: 'selected', model: 'mapped', reported_model: 'actual',
    mint_status: 200, continue_status: 200, new_ticket: true, verdict: 'degraded',
    mint_completed: true, continue_completed: true, mint_latency_ms: 100, continue_latency_ms: 140,
    latency_ms: 240, finished_at: '2026-11-19T04:00:01Z',
  }

  it('按账号发送模型和取消信号，从响应中提取可展示的观察值', async () => {
    const post = vi.spyOn(apiClient, 'post').mockResolvedValue({ data: { ...observed, detail: 'private upstream content', state: 'private state' } })
    const signal = new AbortController().signal
    const data = await runIQStateProbe(22, { model_id: 'selected' }, signal)
    expect(post).toHaveBeenCalledWith('/admin/providers/22/state-probe', { model_id: 'selected' }, { signal, timeout: 0 })
    expect(data).toMatchObject(observed)
    expect(JSON.stringify(data)).not.toContain('private')
  })

  it('拒绝其他账号、无效状态码和缺失结论的响应', async () => {
    const post = vi.spyOn(apiClient, 'post')
    for (const change of [{ account_id: 23 }, { mint_status: 1999 }, { verdict: '' }]) {
      post.mockResolvedValueOnce({ data: { ...observed, ...change } })
      await expect(runIQStateProbe(22, { model_id: 'selected' }, new AbortController().signal)).rejects.toThrow('IQ_INVALID_RESULT')
    }
  })
})

describe('账号 ModelTrace 事件流', () => {
  it('在分片与心跳之间恢复 UTF-8 消息并等待终态，使用当前管理员和指定账号', async () => {
    localStorage.setItem('auth_token', 'fixture')
    const fetch = mockStream(': ping\r\n\r\n' + chunk('modeltrace_start', { challenges: [sample.challenge], bank }) + chunk('modeltrace_sample', sample) + chunk('modeltrace_complete', result))
    const onEvent = vi.fn()
    const signal = new AbortController().signal
    await expect(runIQTrace(12, { model_id: 'selected', reasoning_effort: 'high' }, signal, onEvent)).resolves.toEqual(result)
    expect(fetch).toHaveBeenCalledWith(expect.stringContaining('/admin/providers/12/iq-test/modeltrace'), expect.objectContaining({
      signal, headers: expect.objectContaining({ Authorization: 'Bearer fixture', [ADMIN_UI_REQUEST_HEADER]: '1' }),
      body: JSON.stringify({ model_id: 'selected', reasoning_effort: 'high' }),
    }))
    expect(onEvent.mock.calls.map(([event]) => event.type)).toContain('modeltrace_sample')
  })

  it.each([chunk('modeltrace_sample', sample), chunk('error', { error: 'private upstream message' }), chunk('modeltrace_complete', {})])('缺失有效终态时返回可展示的错误', async input => {
    mockStream(input)
    const promise = runIQTrace(12, { model_id: 'selected', reasoning_effort: '' }, new AbortController().signal, vi.fn())
    await expect(promise).rejects.toThrow()
    await expect(promise).rejects.not.toThrow('private upstream message')
  })

  it('流尚未打开时读取有限的失败类别', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(Response.json({ reason: 'IQ_TEST_UNSUPPORTED', message: 'private' }, { status: 400 })))
    await expect(runIQTrace(12, { model_id: 'a', reasoning_effort: '' }, new AbortController().signal, vi.fn())).rejects.toMatchObject({
      status: 400, reason: 'IQ_TEST_UNSUPPORTED', message: 'IQ_REQUEST_FAILED',
    })
  })
})
