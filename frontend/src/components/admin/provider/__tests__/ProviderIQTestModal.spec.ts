import { flushPromises, mount } from '@vue/test-utils'
import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { Provider } from '@/types'
import ProviderIQTestModal from '../ProviderIQTestModal.vue'

const api = vi.hoisted(() => ({
  runIQQuestion: vi.fn(), runIQQuality: vi.fn(), runIQStateProbe: vi.fn(), runIQTrace: vi.fn(),
  getAvailableModels: vi.fn(), listQuestionDefaults: vi.fn(), saveAs: vi.fn(),
}))
vi.mock('file-saver', () => ({ saveAs: api.saveAs }))
const viewer = ref({ id: 7 })
vi.mock('@/api/admin/providerIQ', async original => ({ ...await original<typeof import('@/api/admin/providerIQ')>(), ...api }))
vi.mock('@/api/admin/providers', () => ({ getAvailableModels: api.getAvailableModels }))
vi.mock('@/api/admin/pelicanTests', () => ({ listQuestionDefaults: api.listQuestionDefaults }))
vi.mock('@/stores/auth', () => ({ useAuthStore: () => ({ get user() { return viewer.value } }) }))
vi.mock('vue-i18n', async original => ({ ...await original<typeof import('vue-i18n')>(), useI18n: () => ({
  locale: ref('zh-Hans'), t: (key: string, params: Record<string, unknown> = {}) => [key, ...Object.values(params)].join(' '),
}) }))

const provider = { id: 12, name: 'Test provider', platform: 'openai', type: 'apikey', credentials: {} } as Provider
const answer = {
  question_kind: 'candy', round_id: 'example', requested_model: 'alpha', mapped_model: 'mapped-alpha',
  reasoning_effort: 'high', prompt: '袋中有几颗糖果？', criteria: ['等于 21'], answer: '<b>21</b>',
  status: 'passed', detail_code: '', started_at: '2026-11-19T02:00:00Z', latency_ms: 74,
}

const probe = {
  account_id: 12, requested_model: 'alpha', model: 'mapped-alpha', reported_model: 'observed-alpha',
  verdict: 'degraded', mint_status: 200, continue_status: 200, new_ticket: true,
  mint_completed: true, continue_completed: true, mint_latency_ms: 19, continue_latency_ms: 22,
  finished_at: '2026-11-19T04:00:00Z', latency_ms: 41,
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(done => { resolve = done })
  return { resolve, promise }
}

function mountModal() {
  return mount(ProviderIQTestModal, {
    props: { show: true, provider },
    global: { stubs: {
      BaseDialog: { props: ['show', 'title', 'subtitle'], template: '<section v-if="show" role="dialog"><h2>{{ title }}</h2><p>{{ subtitle }}</p><slot name="header-icon"/><slot/><slot name="footer"/></section>' },
      Select: { props: ['modelValue', 'options', 'disabled'], emits: ['update:modelValue'], template: '<select :value="modelValue" :disabled="disabled" @change="$emit(\'update:modelValue\', $event.target.value)"><option v-for="o in options" :key="o.value" :value="o.value">{{ o.label }}</option></select>' },
      RouterLink: { props: ['to'], template: '<a :data-route="JSON.stringify(to)"><slot/></a>' },
      Icon: true, PlatformIcon: true, ProviderIcon: true,
    } },
  })
}

beforeEach(() => {
  vi.resetAllMocks()
  localStorage.clear()
  viewer.value = { id: 7 }
  api.getAvailableModels.mockResolvedValue([{ id: 'alpha', display_name: 'Alpha' }, { id: 'beta', display_name: 'Beta' }])
  api.listQuestionDefaults.mockResolvedValue([
    { question_kind: 'candy', question_name: '糖果', question_prompt: answer.prompt, expected_answer: '21' },
    { question_kind: 'pelican', question_name: '鹈鹕', question_prompt: '画一只鹈鹕', expected_answer: '' },
  ])
  api.runIQQuestion.mockResolvedValue(answer)
})

describe('账号智商测试', () => {
  it('在单独的探针页运行指定账号和模型，展示协议观察、定时入口及可回看的结果', async () => {
    api.runIQStateProbe.mockResolvedValueOnce(probe).mockResolvedValueOnce({ ...probe, new_ticket: false, verdict: 'healthy' })
    const view = mountModal()
    await view.setProps({ provider: { ...provider, type: 'oauth' } })
    await flushPromises()
    await view.get('[data-testid="iq-effort"]').setValue('high')
    await view.get('[data-testid="iq-mode-probe"]').trigger('click')
    expect(view.find('[data-testid="iq-mode-quality"]').exists()).toBe(true)
    expect(view.find('[data-testid="iq-effort"]').exists()).toBe(false)
    expect(view.find('[data-testid="iq-prompt"]').exists()).toBe(false)
    expect(JSON.parse(view.get('[data-testid="iq-schedule"]').attributes('data-route'))).toEqual({
      path: '/admin/pelican-tests', query: { provider_id: '12', model_id: 'alpha', question_kind: 'state_probe' },
    })
    await view.get('[data-testid="iq-run"]').trigger('click')
    await flushPromises()
    expect(api.runIQStateProbe).toHaveBeenCalledWith(12, { model_id: 'alpha' }, expect.any(AbortSignal))
    expect(api.runIQQuality).not.toHaveBeenCalled()
    expect(view.get('[data-testid="iq-probe-result"]').text()).toContain('pelicanTests.probe.status.suspected')
    expect(view.get('[data-testid="iq-probe-new-state"]').text()).toBe('providerIQ.probe.yes')
    expect(view.get('[data-testid="iq-probe-first-status"]').text()).toBe('200')
    expect(view.get('[data-testid="iq-probe-next-status"]').text()).toBe('200')
    expect(view.get('[data-testid="iq-probe-first-complete"]').text()).toBe('providerIQ.probe.received')
    expect(view.get('[data-testid="iq-probe-next-duration"]').text()).toContain('22')
    expect(view.findAll('time').some(node => node.attributes('datetime') === probe.finished_at)).toBe(true)
    for (const detail of ['alpha', 'mapped-alpha', 'observed-alpha', '41']) expect(view.text()).toContain(detail)
    await view.get('[data-testid="iq-run"]').trigger('click')
    await flushPromises()
    expect(view.get('[data-testid="iq-probe-new-state"]').text()).toBe('providerIQ.probe.no')
    await view.setProps({ show: false })
    await view.setProps({ show: true })
    await flushPromises()
    await view.get('[data-testid="iq-model"]').setValue('beta')
    await view.get('[data-testid="iq-history"]').trigger('click')
    expect(view.findAll('[data-testid="iq-history-item"]')).toHaveLength(2)
    expect(view.get('[data-testid="iq-history-item"]').text()).toContain('pelicanTests.probe.status.passed')
    await view.get('[data-testid="iq-history-item"]').trigger('click')
    expect(view.text()).toContain('observed-alpha')
    expect(JSON.parse(view.get('[data-testid="iq-schedule"]').attributes('data-route')).query.model_id).toBe('alpha')
    view.unmount()
  })

  it('未完成续接时展示失败分类和未知观察值，取消或换账号后忽略迟到的结果', async () => {
    const delayed = deferred<typeof probe>()
    const leaving = deferred<typeof probe>()
    api.runIQStateProbe.mockResolvedValueOnce({ ...probe, verdict: 'inconclusive', reported_model: '', failure: 'rate_limited', mint_status: 429, continue_status: 0,
      mint_completed: false, continue_completed: false, mint_latency_ms: 19, continue_latency_ms: 0, new_ticket: false })
      .mockReturnValueOnce(delayed.promise).mockReturnValueOnce(leaving.promise)
    const view = mountModal()
    await view.setProps({ provider: { ...provider, type: 'oauth' } })
    await flushPromises()
    await view.get('[data-testid="iq-mode-probe"]').trigger('click')
    await view.get('[data-testid="iq-run"]').trigger('click')
    await flushPromises()
    expect(view.text()).toContain('pelicanTests.probe.reason.rate_limited')
    expect(view.text()).toContain('pelicanTests.probe.status.inconclusive')
    expect(view.get('[data-testid="iq-probe-first-status"]').text()).toBe('429')
    expect(view.get('[data-testid="iq-probe-next-status"]').text()).toBe('providerIQ.probe.noResponse')
    expect(view.get('[data-testid="iq-probe-new-state"]').text()).toBe('providerIQ.probe.unobserved')
    expect(view.get('[data-testid="iq-probe-next-complete"]').text()).toBe('providerIQ.probe.unobserved')
    await view.get('[data-testid="iq-run"]').trigger('click')
    const signal = api.runIQStateProbe.mock.calls[1][2]
    await view.get('[data-testid="iq-cancel"]').trigger('click')
    expect(signal.aborted).toBe(true)
    delayed.resolve(probe)
    await flushPromises()
    expect(view.find('[data-testid="iq-probe-result"]').exists()).toBe(false)
    await view.get('[data-testid="iq-run"]').trigger('click')
    await view.setProps({ provider: { ...provider, id: 14, type: 'oauth' } })
    expect(api.runIQStateProbe.mock.calls[2][2].aborted).toBe(true)
    leaving.resolve(probe)
    await flushPromises()
    await view.get('[data-testid="iq-history"]').trigger('click')
    expect(view.find('[data-testid="iq-history-item"]').exists()).toBe(false)
    expect(JSON.parse(localStorage.getItem('tokenrouter_provider_iq:7:12') || '[]')).toHaveLength(1)
    view.unmount()
  })

  it('续接响应为 HTTP 200 且无完成事件时展示两步各自的耗时和完成情况', async () => {
    api.runIQStateProbe.mockResolvedValueOnce({ ...probe, verdict: 'inconclusive', failure: 'stream_error', continue_completed: false, new_ticket: false })
    const view = mountModal()
    await view.setProps({ provider: { ...provider, type: 'oauth' } })
    await flushPromises()
    await view.get('[data-testid="iq-mode-probe"]').trigger('click')
    await view.get('[data-testid="iq-run"]').trigger('click')
    await flushPromises()
    expect(view.get('[data-testid="iq-probe-next-status"]').text()).toBe('200')
    expect(view.get('[data-testid="iq-probe-next-complete"]').text()).toBe('providerIQ.probe.notReceived')
    expect(view.get('[data-testid="iq-probe-first-duration"]').text()).toContain('19')
    expect(view.get('[data-testid="iq-probe-next-duration"]').text()).toContain('22')
    expect(view.text()).toContain('pelicanTests.probe.reason.stream_error')
    view.unmount()
  })

  it('不适用的账号及映射到非文本模型的配置会显示说明并隐藏探针计划入口', async () => {
    const view = mountModal()
    const cases: Partial<Provider>[] = [
      { type: 'apikey' }, { type: 'setup-token' }, { type: 'oauth', platform: 'anthropic' }, { type: 'oauth', parent_provider_id: 5 },
      { type: 'oauth', credentials: { auth_mode: 'AgentIdentity' } }, { type: 'oauth', credentials: { openai_auth_mode: 'personal_access_token' } },
      { type: 'oauth', credentials: { upstream_protocols: [] } }, { type: 'oauth', credentials: { model_mapping: { 'al*': 'gpt-image-2' } } },
      { type: 'oauth', credentials: { model_mapping: { alpha: 'openai/gpt-4o-mini-transcribe' } } },
      { type: 'oauth', credentials: { model_mapping: { alpha: 'text-moderation-latest' } } },
    ]
    for (const [i, overrides] of cases.entries()) {
      await view.setProps({ provider: { ...provider, id: 60 + i, ...overrides } })
      await flushPromises()
      await view.get('[data-testid="iq-mode-probe"]').trigger('click')
      expect(view.get('[data-testid="iq-run"]').attributes('disabled')).toBeDefined()
      expect(view.find('[data-testid="iq-schedule"]').exists()).toBe(false)
      expect(view.find('[data-testid="iq-probe-unsupported"]').exists()).toBe(true)
    }
    view.unmount()
  })

  it('模型列表为空时可用探针默认模型，探针自己的模型名限制不影响答题', async () => {
    api.getAvailableModels.mockResolvedValueOnce([])
    api.runIQStateProbe.mockResolvedValueOnce({ ...probe, requested_model: 'gpt-6-astra' })
    const view = mountModal()
    await view.setProps({ provider: { ...provider, type: 'oauth' } })
    await flushPromises()
    await view.get('[data-testid="iq-mode-probe"]').trigger('click')
    expect(view.get('[data-testid="iq-run"]').attributes('disabled')).toBeUndefined()
    expect(JSON.parse(view.get('[data-testid="iq-schedule"]').attributes('data-route')).query.model_id).toBe('gpt-6-astra')
    await view.get('[data-testid="iq-run"]').trigger('click')
    await flushPromises()
    expect(api.runIQStateProbe).toHaveBeenCalledWith(12, { model_id: 'gpt-6-astra' }, expect.any(AbortSignal))
    view.getComponent('[data-testid="iq-model"]').vm.$emit('update:modelValue', 'a'.repeat(129))
    await flushPromises()
    expect(view.get('[data-testid="iq-run"]').attributes('disabled')).toBeUndefined()
    expect(view.find('[data-testid="iq-schedule"]').exists()).toBe(false)
    expect(view.text()).toContain('providerIQ.probe.longScheduleModel')
    view.getComponent('[data-testid="iq-model"]').vm.$emit('update:modelValue', 'a'.repeat(256))
    await flushPromises()
    expect(view.get('[data-testid="iq-run"]').attributes('disabled')).toBeDefined()
    expect(view.text()).toContain('providerIQ.probe.modelFormat')
    await view.get('[data-testid="iq-mode-question"]').trigger('click')
    expect(view.get('[data-testid="iq-run"]').attributes('disabled')).toBeUndefined()
    view.unmount()
  })

  it('向所选账号并行提交同一题面，将完成的作答作为纯文本显示并按管理员和账号保存', async () => {
    const view = mountModal()
    await flushPromises()
    await view.get('[data-testid="iq-effort"]').setValue('high')
    await view.get('[data-testid="iq-count"]').setValue('3')
    await view.get('[data-testid="iq-run"]').trigger('click')
    await flushPromises()
    expect(api.runIQQuestion).toHaveBeenCalledTimes(3)
    const [, request] = api.runIQQuestion.mock.calls[0]
    expect(request).toMatchObject({ model_id: 'alpha', question_kind: 'candy', reasoning_effort: 'high', prompt: answer.prompt, expected_answer: '21' })
    expect(request.round_id).toBeTruthy()
    for (const [id, input] of api.runIQQuestion.mock.calls) {
      expect(id).toBe(provider.id)
      expect(input).toEqual(request)
    }
    expect(view.text()).toContain('<b>21</b>')
    expect(view.find('b').exists()).toBe(false)
    expect(view.text()).toContain('mapped-alpha')
    expect(JSON.parse(view.get('[data-testid="iq-schedule"]').attributes('data-route')).query).toMatchObject({ provider_id: '12', model_id: 'alpha', question_kind: 'candy', reasoning_effort: 'high' })
    await view.setProps({ show: false })
    await view.setProps({ show: true })
    await view.get('[data-testid="iq-history"]').trigger('click')
    await view.get('[data-testid="iq-history-item"]').trigger('click')
    expect(view.text()).toContain('<b>21</b>')
    viewer.value = { id: 9 }
    await flushPromises()
    await view.get('[data-testid="iq-history"]').trigger('click')
    expect(view.find('[data-testid="iq-history-item"]').exists()).toBe(false)
    view.unmount()
  })

  it('取消、关闭和切换账号中止在途测试，迟到的结果不会覆盖下一轮', async () => {
    const old = deferred<typeof answer>()
    api.runIQQuestion.mockReturnValueOnce(old.promise).mockResolvedValue(answer)
    const view = mountModal()
    await flushPromises()
    await view.get('[data-testid="iq-run"]').trigger('click')
    const oldSignal = api.runIQQuestion.mock.calls[0][2]
    await view.get('[data-testid="iq-cancel"]').trigger('click')
    expect(oldSignal.aborted).toBe(true)
    await view.get('[data-testid="iq-run"]').trigger('click')
    old.resolve({ ...answer, answer: '忽略过期回答' })
    await flushPromises()
    expect(view.text()).toContain('<b>21</b>')
    expect(view.text()).not.toContain('忽略过期回答')
    const leaving = deferred<typeof answer>()
    api.runIQQuestion.mockReturnValueOnce(leaving.promise)
    await view.get('[data-testid="iq-run"]').trigger('click')
    const leavingSignal = api.runIQQuestion.mock.calls.at(-1)![2]
    await view.setProps({ provider: { ...provider, id: 20, name: 'Another provider' } })
    expect(leavingSignal.aborted).toBe(true)
    leaving.resolve(answer)
    await flushPromises()
    await view.get('[data-testid="iq-history"]').trigger('click')
    expect(view.find('[data-testid="iq-history-item"]').exists()).toBe(false)
    view.unmount()
  })

  it('可提交无参考答案的自定义题，下载单份回答和整轮结果时使用纯文本和 JSON', async () => {
    const view = mountModal()
    await flushPromises()
    await view.get('[data-testid="iq-kind"]').setValue('custom_text')
    expect(view.get('[data-testid="iq-run"]').attributes('disabled')).toBeDefined()
    await view.get('[data-testid="iq-prompt"]').setValue('请把这段文字分成两行。')
    await view.get('[data-testid="iq-count"]').setValue('2')
    await view.get('[data-testid="iq-run"]').trigger('click')
    await flushPromises()
    expect(api.runIQQuestion.mock.calls[0][1]).toMatchObject({ prompt: '请把这段文字分成两行。', question_kind: 'custom_text', expected_answer: '' })
    expect(view.get('[data-testid="iq-schedule"]').attributes('data-route')).not.toContain('这段文字')
    await view.get('[data-testid="iq-download-answer"]').trigger('click')
    await view.get('[data-testid="iq-download-round"]').trigger('click')
    const files = api.saveAs.mock.calls
    expect(files[0][0].type).toContain('text/plain')
    expect(files[0][1]).toMatch(/\.txt$/)
    expect(files[1][0].type).toContain('application/json')
    expect(files[1][1]).toMatch(/\.json$/)
    view.unmount()
  })

  it('指纹记录以实际完成事件为准，显示匹配结果、支持状态和超时', async () => {
    const trace = { requested_model: 'alpha', status: 'complete', created_at: answer.started_at, disagreement: false,
      samples: [], bank: { sha256: 'ab', algorithm: 'test', models: ['Candidate model'] },
      analysis: { prediction: 'Candidate model', probability: 0.85, used_outputs: 3, candidates: [], families: [] } }
    api.runIQTrace.mockResolvedValueOnce(trace).mockRejectedValueOnce({ status: 504 })
    const view = mountModal()
    await flushPromises()
    await view.get('[data-testid="iq-mode-modeltrace"]').trigger('click')
    await view.get('[data-testid="iq-run"]').trigger('click')
    await flushPromises()
    expect(view.text()).toContain('Candidate model')
    expect(view.text()).toContain('85.0%')
    await view.get('[data-testid="iq-run"]').trigger('click')
    await flushPromises()
    expect(view.text()).toContain('providerIQ.timeout')
    await view.get('[data-testid="iq-history"]').trigger('click')
    expect(view.findAll('[data-testid="iq-history-item"]')).toHaveLength(1)
    await view.setProps({ provider: { ...provider, id: 88, platform: 'anthropic', type: 'setup-token' } })
    await flushPromises()
    await view.get('[data-testid="iq-mode-modeltrace"]').trigger('click')
    expect(view.get('[data-testid="iq-run"]').attributes('disabled')).toBeUndefined()
    await view.setProps({ provider: { ...provider, id: 101, platform: 'openai', type: 'setup-token' } })
    await flushPromises()
    await view.get('[data-testid="iq-mode-modeltrace"]').trigger('click')
    expect(view.get('[data-testid="iq-run"]').attributes('disabled')).toBeDefined()
    await view.setProps({ provider: { ...provider, id: 89, type: 'oauth', credentials: { auth_mode: 'agentIdentity' } } })
    await flushPromises()
    await view.get('[data-testid="iq-mode-modeltrace"]').trigger('click')
    expect(view.get('[data-testid="iq-run"]').attributes('disabled')).toBeDefined()
    await view.setProps({ provider: { ...provider, id: 90, credentials: { model_mapping: { 'a*': 'gpt-image-2' } } } })
    await flushPromises()
    await view.get('[data-testid="iq-mode-modeltrace"]').trigger('click')
    expect(view.text()).toContain('providerIQ.traceTextOnly')
    await view.setProps({ provider: { ...provider, id: 91, credentials: { upstream_protocols: ['openai_images_generations'] } } })
    await flushPromises()
    await view.get('[data-testid="iq-mode-modeltrace"]').trigger('click')
    expect(view.text()).toContain('providerIQ.traceProtocol')
    view.unmount()
  })

  it('独立显示可核对的质量结论，只在指纹收到结束事件后保存结果', async () => {
    api.runIQQuality.mockResolvedValue({ requested_model: 'alpha', reasoning_effort: 'default', verdict: 'mismatch', passed: 1, total: 2,
      samples: [answer, { ...answer, answer: '18', status: 'suspected' }], started_at: answer.started_at, latency_ms: 425 })
    api.runIQTrace.mockRejectedValue(new Error('raw upstream content'))
    const view = mountModal()
    await flushPromises()
    await view.get('[data-testid="iq-mode-quality"]').trigger('click')
    await view.get('[data-testid="iq-run"]').trigger('click')
    await flushPromises()
    expect(api.runIQQuality).toHaveBeenCalledWith(12, { model_id: 'alpha', reasoning_effort: '' }, expect.any(AbortSignal))
    expect(view.text()).toContain('providerIQ.qualityVerdict.mismatch')
    expect(view.text()).toContain('18')
    await view.get('[data-testid="iq-mode-modeltrace"]').trigger('click')
    await view.get('[data-testid="iq-run"]').trigger('click')
    await flushPromises()
    expect(view.find('[role="alert"]').exists()).toBe(true)
    expect(view.text()).not.toContain('raw upstream content')
    await view.get('[data-testid="iq-history"]').trigger('click')
    expect(view.findAll('[data-testid="iq-history-item"]')).toHaveLength(1)
    await view.setProps({ provider: { ...provider, platform: 'grok', id: 99 } })
    await flushPromises()
    await view.get('[data-testid="iq-mode-modeltrace"]').trigger('click')
    expect(view.get('[data-testid="iq-run"]').attributes('disabled')).toBeDefined()
    view.unmount()
  })
})
