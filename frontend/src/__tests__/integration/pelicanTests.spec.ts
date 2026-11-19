import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { createPinia } from 'pinia'
import { clearPelicanVisibility, pelicanVisibility, readPelicanVisibility, setPelicanVisibility } from '@/composables/usePelicanVisibility'
import { extractPelicanMarkup } from '@/components/pelican/pelicanMarkup'
import enPelican from '@/i18n/locales/en/pelicanTests'
import zhPelican from '@/i18n/locales/zh/pelicanTests'
import AdminPelicanTestsView from '@/views/admin/PelicanTestsView.vue'
import UserPelicanTestsView from '@/views/user/PelicanTestsView.vue'
import { createPelicanFrame, readPelicanCanvas } from '@/components/pelican/pelicanFrame'

const language = ref('zh-Hans')
const http = vi.hoisted(() => ({
  get: vi.fn(),
  post: vi.fn(),
  put: vi.fn(),
  delete: vi.fn(),
}))
vi.mock('@/api/client', () => ({ apiClient: http }))
vi.mock('@/utils/format', async original => ({
  ...await original<typeof import('@/utils/format')>(),
  formatRelativeTime: (date: string) => date,
}))
vi.mock('vue-i18n', async original => ({
  ...await original<typeof import('vue-i18n')>(),
  useI18n: () => ({
    locale: language,
    t: (key: string, params?: Record<string, unknown>) => [key, ...Object.values(params || {})].join(' '),
  }),
}))

enableAutoUnmount(afterEach)

const plan = {
  id: 8, group_id: 2, group_name: 'Group A', provider_id: 3, provider_name: 'Provider A',
  model_id: 'model-a', question_kind: 'pelican', cron_expression: '0 9 * * *', enabled: false,
  run_state: 'idle', last_run_at: null, next_run_at: null,
}
const result = {
  id: 26, model_id: 'model-a', question_kind: 'pelican', status: 'pending_review',
  tested_at: '2026-11-18T09:00:00Z', latency_ms: 520, detail_code: '',
}
const detail = {
  ...result, prompt: 'Draw a pelican', criteria: ['Two wheels'], provider_name: 'Provider A',
  answer: '<main><svg xmlns="http://www.w3.org/2000/svg"><circle r="2"/></svg ></main>',
}

const builtinQuestions = [
  { question_kind: 'pelican', question_name: '鹈鹕骑车', question_prompt: '用 SVG 绘制骑自行车的鹈鹕。', expected_answer: '' },
  { question_kind: 'candy', question_name: '糖果谜题', question_prompt: '数一数糖果。', expected_answer: '21' },
]

const showcase = { enabled: true, api_enabled: false, auto_cleanup: true, max_items: 20, retention_days: 7 }

const global = {
  plugins: [createPinia()],
  stubs: {
    AppLayout: { template: '<main><slot name="page-heading-actions"/><slot/></main>' },
    BaseDialog: defineComponent({
      props: ['show', 'title', 'subtitle'],
      template: '<section v-if="show" role="dialog"><h2>{{ title }}</h2><p>{{ subtitle }}</p><slot/><slot name="footer"/></section>',
    }),
    Icon: true,
    ModelIcon: true,
    RouterLink: true,
    teleport: true,
  },
}

// 测试从页面入口使用实际 API 客户端函数，仅替换 HTTP 传输。
function mockReads(read: (path: string) => unknown, defaults: () => unknown = () => builtinQuestions, settings: () => unknown = () => showcase) {
  http.get.mockImplementation(async (path: string) => ({
    data: await (path === '/admin/pelican-showcase/settings' ? settings() : path.endsWith('/question-defaults') ? defaults() : read(path)),
  }))
}

beforeEach(() => {
  vi.resetAllMocks()
  clearPelicanVisibility()
  setPelicanVisibility({ enabled: true, api_enabled: false })
  language.value = 'zh-Hans'
})

describe('作品展示设置', () => {
  it('读取展示状态合并同账号的并发请求，旧账号响应无法恢复新账号已关闭的开关', async () => {
    let finishOld!: (value: unknown) => void
    http.get.mockReturnValueOnce(new Promise(resolve => { finishOld = resolve }))
      .mockResolvedValueOnce({ data: { enabled: false, api_enabled: true } })
    const first = readPelicanVisibility(8, true)
    const same = readPelicanVisibility(8, true)
    expect(http.get).toHaveBeenCalledTimes(1)
    const signal = http.get.mock.calls[0][1].signal
    await readPelicanVisibility(9, true)
    expect(signal.aborted).toBe(true)
    finishOld({ data: { enabled: true, api_enabled: true } })
    expect(await first).toBeNull()
    expect(await same).toBeNull()
    expect(pelicanVisibility.value).toEqual({ enabled: false, api_enabled: false })
  })

  it('展示 API 状态取自已保存设置，完整提交并处理数值范围和保留天数', async () => {
    mockReads(() => [plan])
    let complete!: (value: unknown) => void
    http.put.mockReturnValue(new Promise(resolve => { complete = resolve }))
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    const card = view.get('[data-testid="showcase-settings"]')
    expect(card.get('[data-testid="showcase-save"]').attributes('disabled')).toBeDefined()
    await card.get('[data-testid="showcase-api-enabled"]').trigger('click')
    await card.get('[data-testid="showcase-api-info"]').trigger('click')
    expect(view.get('[role="dialog"]').text()).toContain('pelicanTests.showcase.apiUnavailable')
    expect(view.get('[role="dialog"]').text()).toContain('/api/v1/public/pelican-showcase')
    expect(view.get('[role="dialog"]').text()).toContain('x-api-key')
    await view.get('[role="dialog"] button[data-testid="close-showcase-api"]').trigger('click')
    await card.get('#showcase-max-items').setValue('101')
    await card.get('form').trigger('submit')
    expect(http.put).not.toHaveBeenCalled()
    expect(card.text()).toContain('pelicanTests.showcase.invalidCount')
    await card.get('#showcase-max-items').setValue('1')
    await card.get('#showcase-retention-days').setValue('91')
    await card.get('form').trigger('submit')
    expect(http.put).not.toHaveBeenCalled()
    expect(card.text()).toContain('pelicanTests.showcase.invalidDays')
    await card.get('[data-testid="showcase-auto-cleanup"]').trigger('click')
    expect(card.find('#showcase-retention-days').exists()).toBe(false)
    await card.get('form').trigger('submit')
    await card.get('form').trigger('submit')
    expect(http.put).toHaveBeenCalledTimes(1)
    const saved = { enabled: true, api_enabled: true, auto_cleanup: false, max_items: 1, retention_days: 7 }
    expect(http.put).toHaveBeenCalledWith('/admin/pelican-showcase/settings', saved)
    expect(card.get('[data-testid="showcase-save"]').attributes('disabled')).toBeDefined()
    complete({ data: saved })
    await flushPromises()
    expect(card.text()).toContain('pelicanTests.showcase.saved')
    await card.get('[data-testid="showcase-enabled"]').trigger('click')
    await card.get('[data-testid="showcase-api-info"]').trigger('click')
    expect(view.get('[role="dialog"]').text()).toContain('pelicanTests.showcase.apiAvailable')
  })

  it('设置读取失败时计划仍可操作，保存失败保留草稿和已保存状态', async () => {
    let retry = false
    mockReads(() => [plan], () => builtinQuestions, () => {
      if (!retry) throw Error('private-host')
      return showcase
    })
    http.put.mockRejectedValue(Error('private-host'))
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    const card = view.get('[data-testid="showcase-settings"]')
    expect(card.text()).toContain('pelicanTests.showcase.loadFailed')
    expect(view.get('[data-testid="run-plan-8"]').attributes('disabled')).toBeUndefined()
    retry = true
    await card.get('[data-testid="retry-showcase-settings"]').trigger('click')
    await flushPromises()
    await card.get('[data-testid="showcase-enabled"]').trigger('click')
    await card.get('form').trigger('submit')
    await flushPromises()
    expect(card.text()).toContain('pelicanTests.showcase.saveFailed')
    expect(card.text()).not.toContain('private-host')
    expect(card.get('[data-testid="showcase-enabled"]').attributes('aria-checked')).toBe('false')
    expect(view.get('[data-testid="run-plan-8"]').attributes('disabled')).toBeUndefined()
  })
})

describe('管理员测试计划', () => {
  it('标准糖果计划可选择全站停调，改答案后隐藏开关', async () => {
    const candy = { ...plan, question_kind: 'candy', provider_id: null }
    mockReads(path => path.endsWith('/plans') ? [candy] : [])
    http.put.mockResolvedValue({ data: candy })
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    await view.get('[data-testid="edit-plan-8"]').trigger('click')
    await flushPromises()
    const dialog = view.get('[role="dialog"]')
    const toggle = '#pelican-suspend-on-wrong-answer'
    expect(dialog.get(toggle).attributes('aria-checked')).toBe('false')
    await dialog.get(toggle).trigger('click')
    await dialog.get('#pelican-expected-answer').setValue('20')
    expect(dialog.find(toggle).exists()).toBe(false)
    await dialog.get('#pelican-expected-answer').setValue('21')
    await dialog.get('form').trigger('submit')
    await flushPromises()
    expect(http.put).toHaveBeenCalledWith('/admin/pelican-tests/plans/8', expect.objectContaining({
      provider_id: null, question_kind: 'candy', suspend_on_wrong_answer: true,
    }))
  })

  it.each([true, false])('探针计划只允许选定可做探针的账号，资格为 %s', async eligible => {
    mockReads(path => {
      if (path === '/admin/groups/options') return [{ id: 2, name: 'Group A' }]
      if (path === '/admin/providers/3') return { id: 3, name: 'Provider A', group_ids: [2] }
      if (path.endsWith('/groups/2/providers')) return [{ id: 3, name: 'Provider A', supports_state_probe: eligible }]
      return []
    })
    const view = mount(AdminPelicanTestsView, {
      props: { scheduleQuery: { provider_id: '3', model_id: 'alias', question_kind: 'state_probe', reasoning_effort: 'high' } },
      global,
    })
    await flushPromises()
    const form = view.get('#pelican-plan-form')
    expect(form.find('#pelican-question-prompt').exists()).toBe(false)
    expect(form.find('#pelican-expected-answer').exists()).toBe(false)
    expect(form.find('#pelican-effort').exists()).toBe(false)
    await form.trigger('submit')
    if (eligible) {
      expect(http.post).toHaveBeenCalledWith('/admin/pelican-tests/plans', {
        provider_id: 3, group_id: 2, model_id: 'alias', question_kind: 'state_probe', cron_expression: '0 9 * * *', enabled: true,
      })
    } else {
      expect(http.post).not.toHaveBeenCalled()
    }
    view.unmount()
  })

  it('从智商测试新建计划时选择当前账号和它所属的分组', async () => {
    mockReads(path => {
      if (path === '/admin/groups/options') return [{ id: 2, name: 'Group A' }, { id: 4, name: 'Group B' }]
      if (path === '/admin/providers/3') return { id: 3, name: 'Provider A', group_ids: [2] }
      if (path.endsWith('/groups/2/providers')) return [{ id: 3, name: 'Provider A' }]
      return []
    })
    const view = mount(AdminPelicanTestsView, {
      props: { scheduleQuery: { provider_id: '3', model_id: 'demo-text', question_kind: 'candy', reasoning_effort: 'high' } },
      global,
    })
    await flushPromises()
    const form = view.get('#pelican-plan-form')
    expect(form.get('#pelican-provider').text()).toContain('Provider A')
    expect(form.get('#pelican-provider').attributes('disabled')).toBeDefined()
    expect(form.get('#pelican-group').text()).toContain('Group A')
    await form.trigger('submit')
    expect(http.post).toHaveBeenCalledWith('/admin/pelican-tests/plans', expect.objectContaining({
      group_id: 2, provider_id: 3, model_id: 'demo-text', question_kind: 'candy', reasoning_effort: 'high',
    }))
  })

  it('列出可搜索的频率预设，编辑自定义 Cron 后切换预设仍可取回表达式', async () => {
    const odd = { ...plan, cron_expression: '12 7 * * 1-5' }
    mockReads(path => {
      if (path.endsWith('/plans')) return [odd, { ...plan, id: 9, cron_expression: '*/30 * * * *' }]
      if (path.endsWith('/providers')) return [{ id: 3, name: 'Provider A' }]
      if (path.endsWith('models-list-candidates')) return { models: [] }
      throw Error(path)
    })
    http.put.mockResolvedValue({ data: odd })
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    expect(view.text()).toContain('pelicanTests.frequency.every30m')
    expect(view.text()).toContain(odd.cron_expression)
    await view.get('[data-testid="edit-plan-8"]').trigger('click')
    await flushPromises()
    expect(view.get('#pelican-frequency').text()).toContain('pelicanTests.frequency.custom')
    expect(view.get<HTMLInputElement>('#pelican-cron').element.value).toBe(odd.cron_expression)
    await view.get('#pelican-cron').setValue('35 18 * * 1-5')
    await view.get('#pelican-frequency').trigger('click')
    expect(view.findAll('[role="option"]').map(o => o.text())).toEqual(
      ['every15m', 'every30m', 'hourly', 'every2h', 'every6h', 'daily', 'custom'].map(k => 'pelicanTests.frequency.' + k),
    )
    await view.get('[role="listbox"] input').setValue('every2h')
    expect(view.findAll('[role="option"]')).toHaveLength(1)
    await view.get('[role="option"]').trigger('click')
    expect(view.find('#pelican-cron').exists()).toBe(false)
    await view.get('#pelican-frequency').trigger('click')
    await view.findAll('[role="option"]').find(o => o.text().endsWith('.custom'))!.trigger('click')
    expect(view.get<HTMLInputElement>('#pelican-cron').element.value).toBe('35 18 * * 1-5')
    await view.get('#pelican-frequency').trigger('click')
    await view.findAll('[role="option"]').find(o => o.text().endsWith('.every6h'))!.trigger('click')
    await view.get('#pelican-plan-form').trigger('submit')
    await flushPromises()
    expect(http.put).toHaveBeenCalledWith('/admin/pelican-tests/plans/8', expect.objectContaining({ cron_expression: '0 */6 * * *' }))
  })

  it('回填两种内置题，新建糖果计划时写入当前题面和编辑后的参考答案', async () => {
    mockReads(path => {
      if (path.endsWith('/plans')) return []
      if (path === '/admin/groups/options') return [{ id: 2, name: 'Group A' }]
      if (path.endsWith('/providers')) return []
      if (path.endsWith('models-list-candidates')) return { models: [] }
      throw Error(path)
    })
    http.post.mockResolvedValue({ data: {} })
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    await view.get('[data-testid="create-plan"]').trigger('click')
    await flushPromises()
    expect(view.get('#pelican-frequency').text()).toContain('pelicanTests.frequency.daily')
    expect(view.find('#pelican-cron').exists()).toBe(false)
    expect(view.get<HTMLTextAreaElement>('#pelican-question-prompt').element.value).toBe(builtinQuestions[0].question_prompt)
    expect(view.find('#pelican-expected-answer').exists()).toBe(false)
    await view.get('#pelican-model').setValue('another-model')
    await view.get('#pelican-group').trigger('click')
    await view.get('[role="option"]').trigger('click')
    await view.findAll('[role="radio"]').find(el => el.text() === 'pelicanTests.kind.candy')!.trigger('click')
    expect(view.get<HTMLInputElement>('#pelican-question-name').element.value).toBe(builtinQuestions[1].question_name)
    expect(view.get<HTMLTextAreaElement>('#pelican-expected-answer').element.value).toBe('21')
    await view.get('#pelican-expected-answer').setValue('27')
    await view.findAll('[role="radio"]').find(el => el.text() === 'pelicanTests.kind.pelican')!.trigger('click')
    expect(view.get<HTMLTextAreaElement>('#pelican-question-prompt').element.value).toBe(builtinQuestions[0].question_prompt)
    await view.findAll('[role="radio"]').find(el => el.text() === 'pelicanTests.kind.candy')!.trigger('click')
    expect(view.get<HTMLTextAreaElement>('#pelican-expected-answer').element.value).toBe('27')
    await view.get('#pelican-plan-form').trigger('submit')
    await flushPromises()
    expect(http.post).toHaveBeenCalledWith('/admin/pelican-tests/plans', {
      ...builtinQuestions[1], expected_answer: '27', group_id: 2, model_id: 'another-model', provider_id: null,
      enabled: true, cron_expression: '0 9 * * *',
    })
  })

  it('保存轮换计划时仅写入已改字段，常用题复制到新计划后固定当前题面', async () => {
    const renamed = { ...plan, id: 12, question_name: '旧题改名' }
    mockReads(path => {
      if (path.endsWith('/plans')) return [plan, renamed]
      if (path === '/admin/groups/options') return [{ id: 2, name: 'Group A' }]
      if (path.endsWith('/providers')) return [{ id: 3, name: 'Provider A' }]
      if (path.endsWith('models-list-candidates')) return { models: [] }
      throw Error(path)
    })
    http.put.mockResolvedValue({ data: plan })
    http.post.mockResolvedValue({ data: {} })
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    await view.get('[data-testid="edit-plan-8"]').trigger('click')
    await flushPromises()
    expect(view.get('#pelican-plan-form').text()).toContain('pelicanTests.rotatingQuestionHint')
    expect(view.get<HTMLTextAreaElement>('#pelican-question-prompt').element.value).toBe(builtinQuestions[0].question_prompt)
    await view.get('#pelican-question-name').setValue('计划名称')
    await view.get('#pelican-plan-form').trigger('submit')
    await flushPromises()
    const saved = http.put.mock.calls[0][1]
    expect(saved.question_name).toBe('计划名称')
    expect(saved.question_prompt || '').toBe('')
    expect(saved.expected_answer || '').toBe('')
    await view.get('[data-testid="create-plan"]').trigger('click')
    await flushPromises()
    await view.get('#pelican-question-template').trigger('click')
    await view.findAll('[role="option"]').find(option => option.text().includes('旧题改名'))!.trigger('click')
    expect(view.get<HTMLTextAreaElement>('#pelican-question-prompt').element.value).toBe(builtinQuestions[0].question_prompt)
    await view.get('#pelican-model').setValue('template-model')
    await view.get('#pelican-group').trigger('click')
    await view.get('[role="option"]').trigger('click')
    await view.get('#pelican-plan-form').trigger('submit')
    await flushPromises()
    expect(http.post.mock.calls[0][1]).toMatchObject({ question_name: '旧题改名', question_prompt: builtinQuestions[0].question_prompt })
    expect(http.get.mock.calls.filter(([url]) => url.endsWith('/question-defaults'))).toHaveLength(1)
  })

  it('题型的默认值按界面语言缓存，切换语言保留管理员已经输入的题面', async () => {
    mockReads(path => path.endsWith('/plans') ? [plan] : path.endsWith('/providers') ? [{ id: 3, name: 'Provider A' }] : { models: [] },
      () => builtinQuestions.map(question => ({ ...question, question_name: language.value + question.question_name })))
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    await view.get('[data-testid="edit-plan-8"]').trigger('click')
    await flushPromises()
    await view.get('#pelican-question-prompt').setValue('我的题目')
    language.value = 'en'
    await flushPromises()
    expect(view.get<HTMLInputElement>('#pelican-question-name').element.value).toBe('en鹈鹕骑车')
    expect(view.get<HTMLTextAreaElement>('#pelican-question-prompt').element.value).toBe('我的题目')
    language.value = 'zh-Hans'
    await flushPromises()
    expect(view.get<HTMLInputElement>('#pelican-question-name').element.value).toBe('zh-Hans鹈鹕骑车')
    expect(http.get.mock.calls.filter(([url]) => url.endsWith('/question-defaults'))).toHaveLength(2)
  })

  it('默认题目读取失败可重试，服务端返回前保存动作不会产生空题面', async () => {
    let fail = true
    mockReads(path => {
      if (path.endsWith('/plans')) return []
      if (path === '/admin/groups/options') return [{ id: 2, name: 'Group A' }]
      if (path.endsWith('/providers')) return []
      if (path.endsWith('models-list-candidates')) return { models: [] }
      throw Error(path)
    }, () => {
      if (fail) throw Error('upstream-internal-host')
      return builtinQuestions
    })
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    await view.get('[data-testid="create-plan"]').trigger('click')
    await flushPromises()
    await view.get('#pelican-group').trigger('click')
    await view.get('[role="option"]').trigger('click')
    await view.get('#pelican-model').setValue('write-model')
    await view.get('#pelican-plan-form').trigger('submit')
    expect(http.post).not.toHaveBeenCalled()
    expect(view.get('[role="alert"]').text()).toContain('pelicanTests.defaultsLoadFailed')
    expect(view.text()).not.toContain('upstream-internal-host')
    fail = false
    await view.get('[data-testid="retry-defaults"]').trigger('click')
    await flushPromises()
    expect(view.get<HTMLTextAreaElement>('#pelican-question-prompt').element.value).toBe(builtinQuestions[0].question_prompt)
  })

  it('从已有文本题复制题面，按 UTF-8 校验自定义题，并独立保存名称、答案和推理强度', async () => {
    const source = {
      ...plan, question_kind: 'custom_text', question_name: '算术', question_prompt: '计算 12 + 5', expected_answer: '17',
      reasoning_effort: 'high',
    }
    mockReads(path => {
      if (path.endsWith('/plans')) return [source]
      if (path === '/admin/groups/options') return [{ id: 2, name: 'Group A' }]
      if (path.endsWith('/providers')) return []
      if (path.endsWith('models-list-candidates')) return { models: [] }
      throw Error(path)
    })
    http.post.mockResolvedValue({ data: {} })
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    await view.get('[data-testid="create-plan"]').trigger('click')
    await flushPromises()
    await view.get('#pelican-group').trigger('click')
    await view.get('[role="option"]').trigger('click')
    await view.get('#pelican-model').setValue('new-model')
    await view.get('#pelican-question-template').trigger('click')
    await view.findAll('[role="option"]').find(option => option.text().includes('算术'))!.trigger('click')
    const prompt = view.get<HTMLTextAreaElement>('#pelican-question-prompt')
    expect(prompt.element.value).toBe(source.question_prompt)
    expect(view.get<HTMLTextAreaElement>('#pelican-expected-answer').element.value).toBe('17')
    await view.get('#pelican-question-name').setValue('算'.repeat(33))
    await view.get('#pelican-plan-form').trigger('submit')
    expect(view.text()).toContain('pelicanTests.invalidQuestionName')
    await view.get('#pelican-question-name').setValue('新算术')
    await prompt.setValue('问'.repeat(2731))
    await view.get('#pelican-plan-form').trigger('submit')
    expect(view.text()).toContain('pelicanTests.invalidQuestionPrompt')
    await prompt.setValue('计算 18 + 5')
    await view.get('#pelican-expected-answer').setValue('答'.repeat(342))
    await view.get('#pelican-plan-form').trigger('submit')
    expect(view.text()).toContain('pelicanTests.invalidExpectedAnswer')
    expect(http.post).not.toHaveBeenCalled()
    await view.get('#pelican-expected-answer').setValue('23')
    await view.get('#pelican-effort').trigger('click')
    await view.findAll('[role="option"]').find(option => option.text() === 'pelicanTests.effort.xhigh')!.trigger('click')
    await view.get('#pelican-plan-form').trigger('submit')
    await flushPromises()
    expect(http.post).toHaveBeenCalledWith('/admin/pelican-tests/plans', {
      group_id: 2, model_id: 'new-model', provider_id: null, enabled: true, cron_expression: '0 9 * * *',
      question_kind: 'custom_text', question_name: '新算术', question_prompt: '计算 18 + 5', expected_answer: '23', reasoning_effort: 'xhigh',
    })
    expect(source.question_name).toBe('算术')
  })

  it('切换定时状态保留自定义题和强度，改为 SVG 题时移除文本答案', async () => {
    const custom = {
      ...plan, question_kind: 'custom_text', question_name: '太阳', question_prompt: '写出恒星名字',
      expected_answer: '太阳', reasoning_effort: 'low',
    }
    mockReads(path => {
      if (path.endsWith('/plans')) return [custom]
      if (path.endsWith('/providers')) return [{ id: 3, name: 'Provider A' }]
      if (path.endsWith('models-list-candidates')) return { models: [] }
      throw Error(path)
    })
    http.put.mockResolvedValue({ data: custom })
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    expect(view.text()).toContain('太阳')
    await view.get('[data-testid="plan-schedule-8"]').trigger('click')
    await flushPromises()
    expect(http.put).toHaveBeenCalledWith('/admin/pelican-tests/plans/8', expect.objectContaining({
      enabled: true, question_name: '太阳', question_prompt: '写出恒星名字', expected_answer: '太阳', reasoning_effort: 'low',
    }))
    await view.get('[data-testid="edit-plan-8"]').trigger('click')
    await flushPromises()
    await view.findAll('[role="radio"]').find(button => button.text() === 'pelicanTests.kind.custom_svg')!.trigger('click')
    expect(view.find('#pelican-expected-answer').exists()).toBe(false)
    await view.get('#pelican-question-prompt').setValue('用 SVG 画太阳')
    await view.get('#pelican-plan-form').trigger('submit')
    await flushPromises()
    const payload = http.put.mock.calls.at(-1)?.[1]
    expect(payload).toMatchObject({ question_kind: 'custom_svg', question_prompt: '用 SVG 画太阳', reasoning_effort: 'low' })
    expect(payload).not.toHaveProperty('expected_answer')
  })

  it('切换分组后忽略迟到的提供商列表，校验手填模型和五字段 cron', async () => {
    let releaseFirst!: (value: unknown) => void
    mockReads(path => {
      if (path.endsWith('/plans')) return []
      if (path === '/admin/groups/options') return [{ id: 2, name: 'Group A' }, { id: 7, name: 'Group B' }]
      if (path.endsWith('/2/providers')) return new Promise(resolve => { releaseFirst = resolve })
      if (path.endsWith('/7/providers')) return [{ id: 17, name: 'Provider B' }]
      if (path.endsWith('models-list-candidates')) return { models: ['catalog-model'] }
      throw Error(path)
    })
    http.post.mockResolvedValue({ data: { ...plan, id: 9 } })
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    await view.get('[data-testid="create-plan"]').trigger('click')
    await flushPromises()

    await view.get('#pelican-group').trigger('click')
    await view.get('[role="option"]').trigger('click')
    await view.get('#pelican-group').trigger('click')
    await view.findAll('[role="option"]').find(option => option.text() === 'Group B')!.trigger('click')
    await flushPromises()
    releaseFirst([{ id: 3, name: 'Provider A' }])
    await flushPromises()

    await view.get('#pelican-provider').trigger('click')
    expect(view.findAll('[role="option"]').map(option => option.text())).not.toContain('Provider A')
    await view.findAll('[role="option"]').find(option => option.text() === 'Provider B')!.trigger('click')
    await view.get('#pelican-model').setValue(' custom-model ')
    await view.get('#pelican-frequency').trigger('click')
    await view.findAll('[role="option"]').find(option => option.text() === 'pelicanTests.frequency.custom')!.trigger('click')
    await view.get('#pelican-cron').setValue('0 * * *')
    await view.get('#pelican-plan-form').trigger('submit')
    expect(http.post).not.toHaveBeenCalled()
    expect(view.text()).toContain('pelicanTests.invalidCron')

    await view.get('#pelican-cron').setValue('15 8 * * 1-5')
    await view.get('#pelican-model').setValue('model-*')
    await view.get('#pelican-plan-form').trigger('submit')
    expect(http.post).not.toHaveBeenCalled()
    expect(view.text()).toContain('pelicanTests.invalidModel')
    await view.get('#pelican-model').setValue('模'.repeat(43))
    await view.get('#pelican-plan-form').trigger('submit')
    expect(http.post).not.toHaveBeenCalled()
    await view.get('#pelican-model').setValue(' custom-model ')
    await view.get('#pelican-plan-form').trigger('submit')
    await flushPromises()
    expect(http.post).toHaveBeenCalledWith('/admin/pelican-tests/plans', {
      group_id: 7, provider_id: 17, model_id: 'custom-model', question_kind: 'pelican',
      question_name: builtinQuestions[0].question_name, question_prompt: builtinQuestions[0].question_prompt,
      cron_expression: '15 8 * * 1-5', enabled: true,
    })
  })

  it('已解绑的提供商需要重新选择，可以用自动调度保存糖果题', async () => {
    mockReads(path => {
      if (path.endsWith('/plans')) return [plan]
      if (path.endsWith('/providers')) return []
      if (path.endsWith('models-list-candidates')) return { models: [] }
      throw Error(path)
    })
    http.put.mockResolvedValue({ data: { ...plan, provider_id: null, question_kind: 'candy' } })
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    await view.get('[data-testid="edit-plan-8"]').trigger('click')
    await flushPromises()
    await view.get('#pelican-plan-form').trigger('submit')
    expect(http.put).not.toHaveBeenCalled()
    expect(view.text()).toContain('pelicanTests.chooseProvider')
    await view.get('#pelican-provider').trigger('click')
    await view.findAll('[role="option"]').find(option => option.text() === 'pelicanTests.autoProvider')!.trigger('click')
    await view.findAll('[role="radio"]').find(button => button.text() === 'pelicanTests.kind.candy')!.trigger('click')
    await view.get('#pelican-plan-form').trigger('submit')
    await flushPromises()
    expect(http.put).toHaveBeenCalledWith('/admin/pelican-tests/plans/8', expect.objectContaining({
      provider_id: null, question_kind: 'candy',
    }))
  })

  it('编辑时锁定分组，保存和删除都只作用于选中的计划', async () => {
    mockReads(path => {
      if (path.endsWith('/plans')) return [plan]
      if (path.endsWith('/providers')) return [{ id: 3, name: 'Provider A' }]
      if (path.endsWith('models-list-candidates')) return { models: [] }
      if (path === '/admin/groups/options') return [{ id: 2, name: 'Group A' }]
      throw Error(path)
    })
    http.put.mockResolvedValue({ data: plan })
    http.delete.mockResolvedValue({ data: null })
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    await view.get('[data-testid="edit-plan-8"]').trigger('click')
    await flushPromises()
    expect(view.get('#pelican-group').attributes('disabled')).toBeDefined()
    await view.get('#pelican-model').setValue('new-model')
    await view.get('#pelican-plan-form').trigger('submit')
    await flushPromises()
    expect(http.put).toHaveBeenCalledWith('/admin/pelican-tests/plans/8', {
      group_id: 2, provider_id: 3, model_id: 'new-model', question_kind: 'pelican',
      cron_expression: plan.cron_expression, enabled: false,
    })
    await view.get('[data-testid="delete-plan-8"]').trigger('click')
    expect(http.delete).not.toHaveBeenCalled()
    await view.get('button.btn-danger').trigger('click')
    await flushPromises()
    expect(http.delete).toHaveBeenCalledWith('/admin/pelican-tests/plans/8')
  })

  it('计划已排队时禁止再次运行，管理员仍可确认删除', async () => {
    mockReads(() => [{ ...plan, run_state: 'queued' }])
    http.delete.mockResolvedValue({ data: null })
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    expect(view.get('[data-testid="run-plan-8"]').attributes('disabled')).toBeDefined()
    await view.get('[data-testid="delete-plan-8"]').trigger('click')
    await view.get('button.btn-danger').trigger('click')
    await flushPromises()
    expect(http.delete).toHaveBeenCalledWith('/admin/pelican-tests/plans/8')
  })

  it('切换定时状态时防止重复提交，成功后立即更新开关', async () => {
    let finish!: (value: unknown) => void
    http.get.mockResolvedValueOnce({ data: [plan] }).mockReturnValue(new Promise(() => {}))
    http.put.mockReturnValue(new Promise(resolve => { finish = resolve }))
    const view = mount(AdminPelicanTestsView, { global })
    await flushPromises()
    const toggle = view.get('[data-testid="plan-schedule-8"]')
    await toggle.trigger('click')
    await toggle.trigger('click')
    expect(http.put).toHaveBeenCalledTimes(1)
    expect(toggle.attributes('disabled')).toBeDefined()
    finish({ data: { ...plan, enabled: true } })
    await flushPromises()
    expect(toggle.attributes('aria-checked')).toBe('true')
    expect(http.put).toHaveBeenCalledWith('/admin/pelican-tests/plans/8', expect.objectContaining({ enabled: true }))
  })

  it('停用定时计划后仍可立即运行，轮询结束后在记录中复核作品', async () => {
    vi.useFakeTimers()
    try {
      let state = 'idle'
      let done = false
      mockReads(path => {
        if (path.endsWith('/plans')) return [{ ...plan, run_state: state, last_run_at: done ? result.tested_at : null }]
        if (path.endsWith('/8/results')) return done ? [{ ...result, question_kind: 'custom_svg', question_name: '夜空' }] : []
        if (path.endsWith('/results/26')) return { ...detail, question_kind: 'custom_svg', question_name: '夜空' }
        throw Error(path)
      })
      http.post.mockImplementation(async () => { state = 'queued'; return { data: {} } })
      http.put.mockResolvedValue({ data: { ...detail, status: 'suspected' } })
      const view = mount(AdminPelicanTestsView, { global })
      await flushPromises()
      await view.get('[data-testid="run-plan-8"]').trigger('click')
      await flushPromises()
      expect(http.post.mock.calls.map(call => call[0])).toEqual(['/admin/pelican-tests/plans/8/run'])
      expect(view.get('[data-testid="run-plan-8"]').attributes('disabled')).toBeDefined()
      expect(view.text()).toContain('pelicanTests.runState.queued')

      state = 'idle'
      done = true
      await vi.advanceTimersByTimeAsync(5000)
      await flushPromises()
      await view.get('[data-testid="open-sample-26"]').trigger('click')
      await flushPromises()
      expect(view.text()).toContain('Two wheels')
      expect(view.text()).toContain('Provider A')
      expect(view.get('img').attributes('src')).toMatch(/^data:image\/svg\+xml/)
      expect(decodeURIComponent(view.get('img').attributes('src'))).not.toContain('<main>')
      expect(view.find('iframe').exists()).toBe(false)
      await view.get('[data-testid="review-suspected"]').trigger('click')
      await flushPromises()
      expect(http.put).toHaveBeenCalledWith('/admin/pelican-tests/results/26/review', { status: 'suspected' })
      expect(view.find('[data-testid="review-passed"]').exists()).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('两次刷新之间完成的定时测试会出现在打开的运行记录里', async () => {
    vi.useFakeTimers()
    try {
      let finished = false
      mockReads(path => {
        if (path.endsWith('/plans')) return [{ ...plan, enabled: true, last_run_at: finished ? result.tested_at : null }]
        if (path.endsWith('/8/results')) return finished ? [result] : []
        throw Error(path)
      })
      const view = mount(AdminPelicanTestsView, { global })
      await flushPromises()
      await view.findAll('button').find(button => button.text() === 'pelicanTests.history')!.trigger('click')
      await flushPromises()
      expect(view.text()).toContain('pelicanTests.noHistory')
      finished = true
      await vi.advanceTimersByTimeAsync(30000)
      await flushPromises()
      expect(view.find('[data-testid="open-sample-26"]').exists()).toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })
})

describe('用户样本', () => {
  it('有绘图作品时在画廊顶部展示一次判读说明，空组、文本、探针及关闭展示时隐藏', async () => {
    let hasDrawings = false
    mockReads(() => [
      { group_id: 1, group_name: '空组', models: [] },
      { group_id: 2, group_name: 'Group A', models: [
        { ...plan, latest: null, artworks: [] },
        { ...plan, question_kind: 'state_probe', latest: { ...result, question_kind: 'state_probe' } },
        { ...plan, question_kind: 'candy', latest: null, answers: [{ ...result, question_kind: 'candy', answer_preview: '21' }] },
      ] },
      ...(hasDrawings ? [3, 4].map(id => ({
        group_id: id, group_name: String(id), models: [{ ...plan, latest: { ...result, id } }],
      })) : []),
    ])
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()
    expect(view.find('[role="note"]').exists()).toBe(false)
    hasDrawings = true
    await view.get('[data-testid="refresh-samples"]').trigger('click')
    await flushPromises()
    const notes = view.findAll('[role="note"]')
    expect(notes.map(note => note.text())).toEqual(['pelicanTests.showcase.artworkNotice'])
    expect(notes[0].element.compareDocumentPosition(view.get('[data-testid="artwork-strip"]').element)
      & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy()
    expect(zhPelican.showcase.artworkNotice).toMatch(/token.*鹈鹕.*腿.*轮子.*不转.*正常.*风格.*没有降智/)
    expect(enPelican.showcase.artworkNotice).toMatch(/token.*legs.*wheels.*normal.*style/i)
    setPelicanVisibility({ enabled: false, api_enabled: true })
    await flushPromises()
    expect(view.find('[role="note"]').exists()).toBe(false)
  })

  it('画廊卡片与样本详情都预览 SVG 外的样式和脚本', async () => {
    const raw = '<!doctype html><html><head><style>.wheel{stroke:red}</style></head><body><svg viewBox="0 0 600 400"><circle class="wheel" r="4"/></svg><script>document.querySelector("circle").setAttribute("r","8")</script></body></html>'
    mockReads(path => path === '/pelican-tests'
      ? [{ group_id: 2, group_name: 'Group A', models: [{ ...plan, latest: result }] }]
      : { ...detail, raw_answer: raw })
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()
    await view.get('[data-testid="open-sample-26"]').trigger('focus')
    await flushPromises()
    const thumbnail = view.get('[data-testid="artwork-strip"] iframe')
    expect(thumbnail.attributes('srcdoc')).toContain('.wheel{stroke:red}')
    expect(thumbnail.attributes('srcdoc')).toContain('document.querySelector')
    expect(thumbnail.element.closest('button')).toBeNull()
    expect(thumbnail.attributes('tabindex')).toBe('-1')
    await view.get('[data-testid="open-sample-26"]').trigger('click')
    await flushPromises()
    const dialog = view.get('[role="dialog"]')
    const frame = dialog.get('iframe')
    expect(frame.attributes('tabindex')).toBeUndefined()
    expect(frame.classes()).not.toContain('pointer-events-none')
    expect(frame.attributes('sandbox')).toBe('allow-scripts allow-pointer-lock')
    expect(dialog.get('details > summary').text()).toContain('pelicanTests.prompt')
    expect(frame.attributes('referrerpolicy')).toBe('no-referrer')
    expect(frame.attributes('srcdoc')).toContain('.wheel{stroke:red}')
    expect(frame.attributes('srcdoc')).toContain('document.querySelector')
    expect(frame.attributes('srcdoc')).toContain("default-src 'none'")
    expect(frame.attributes('srcdoc')).toContain("connect-src 'none'")
    expect(dialog.find('script, svg').exists()).toBe(false)
    await dialog.get('[data-testid="pelican-show-source"]').trigger('click')
    expect(dialog.get('[data-testid="pelican-original-source"]').text()).toBe(raw)
    expect(dialog.get('[data-testid="pelican-download-source"]').attributes('download')).toBe('pelican-26.html')
    expect(decodeURIComponent(dialog.get('[data-testid="pelican-download-source"]').attributes('href').split(',')[1])).toBe(raw)
  })

  it('裸 SVG 后的说明不占画布，画布外的样式与动画仍进入作品', () => {
    const svg = '<svg viewBox="0 0 600 300"><path d="M5 5h75"/></svg>'
    expect(extractPelicanMarkup('作品如下。\n' + svg + '\n这是完成后的鹈鹕。')).toBe(svg)
    const animated = '<style>path{stroke:blue}</style>' + svg + '<script>document.querySelector("path").style.fill="gold"</script>'
    expect(extractPelicanMarkup(animated + '\n完成。')).toBe(animated)
  })

  it('有多份作品时展示模型最后写出的文档，末份残缺时等待完整作品', () => {
    const first = '<html><body><svg><title>草稿</title><circle r="5"/></svg></body></html>'
    const last = '<!DOCTYPE html><html><body><canvas></canvas><script>const literal = "<html> "</script></body></html>'
    for (const wrap of [(s: string) => s, (s: string) => '```html\n' + s + '\n```']) {
      expect(extractPelicanMarkup(wrap(first) + '\n' + wrap(last))).toBe(last)
      expect(extractPelicanMarkup(wrap(first) + '\n' + wrap(last.slice(0, -7)))).toBe('')
    }
    const sketch = '```svg\n<svg><circle r="2"/></svg>\n```'
    expect(extractPelicanMarkup(sketch + '\n最终作品如下。\n' + last)).toBe(last)
    expect(extractPelicanMarkup(sketch + '\n最终作品如下。\n' + last.slice(0, -7))).toBe('')
    expect(extractPelicanMarkup(sketch + '\n这幅画使用 <svg> 标记。')).toBe('<svg><circle r="2"/></svg>')
    expect(extractPelicanMarkup('<svg><path d="M1 2h3"/></svg><script>draw(')).toBe('')
  })

  it.each([
    '<html><body><svg><path d="M1 2h3"/></svg><script>const ending = "</html>"',
    '<!doctype html><html><body><svg><circle r="5"/></svg><!-- </html> -->',
    '<svg viewBox="0 0 100 100"><svg><circle r="4"/></svg>',
    '<svg><circle r="5"/></svg><script',
  ])('历史样本有未闭合的原文时，卡片和详情都没有作品预览', async raw => {
    mockReads(path => path === '/pelican-tests'
      ? [{ group_id: 2, group_name: 'Group A', models: [{ ...plan, latest: result }] }]
      : { ...detail, raw_answer: raw })
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()
    await view.get('[data-testid="open-sample-26"]').trigger('focus')
    await flushPromises()
    await view.get('[data-testid="open-sample-26"]').trigger('click')
    await flushPromises()
    expect(view.find('[data-testid="pelican-artwork-preview"]').exists()).toBe(false)
    expect(view.get('[role="dialog"]').text()).toContain('pelicanTests.reason.incomplete')
  })

  it('作品移出视口时卸载动画，重新进入时用缓存预览 HTML 画布', async () => {
    let intersect!: (visible: boolean) => void
    class Observer {
      constructor(callback: (entries: { isIntersecting: boolean }[]) => void) {
        intersect = visible => callback([{ isIntersecting: visible }])
      }
      observe() {}
      disconnect() {}
    }
    vi.stubGlobal('IntersectionObserver', Observer)
    try {
      const raw = '<!doctype html><html><body><canvas width="1200" height="900"></canvas></body></html>'
      mockReads(path => path === '/pelican-tests'
        ? [{ group_id: 2, group_name: 'Group A', models: [{ ...plan, latest: result }] }]
        : { ...detail, answer: '', raw_answer: raw })
      const view = mount(UserPelicanTestsView, { global })
      await flushPromises()
      expect(view.find('iframe').exists()).toBe(false)
      intersect(true)
      await flushPromises()
      expect(view.get('iframe').attributes('srcdoc')).toContain('canvas')
      intersect(false)
      await flushPromises()
      expect(view.find('iframe').exists()).toBe(false)
      intersect(true)
      await flushPromises()
      expect(view.find('iframe').exists()).toBe(true)
      expect(http.get.mock.calls.map(call => call[0])).toEqual(['/pelican-tests', '/pelican-tests/results/26'])
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('预览去掉导航标签并在隔离框架中为内联动画加 nonce', () => {
    const nonce = document.createElement('script')
    nonce.nonce = 'page-nonce'
    document.head.appendChild(nonce)
    try {
      const raw = '作品如下。\n```html\n<html><head><base href="https://fixture.invalid"><meta http-equiv="refresh" content="0;url=https://fixture.invalid"><style>.wheel{stroke:red}</style></head><body><svg><script href="https://fixture.invalid/remote.js" nonce="author"></script></svg><script src="https://fixture.invalid/remote.js"></script><script>window.frame=1</script></body></html>\n```'
      const { srcdoc } = createPelicanFrame(extractPelicanMarkup(raw))
      const parser = new DOMParser()
      const wrapper = parser.parseFromString(srcdoc, 'text/html')
      expect(wrapper.head.firstElementChild?.getAttribute('content')).toContain("frame-src 'none'")
      const frame = wrapper.querySelector('template')!.content.querySelector('iframe')!
      expect(frame.getAttribute('sandbox')).toBe('allow-scripts')
      const parsed = parser.parseFromString(frame.srcdoc, 'text/html')
      expect(parsed.querySelector('base,meta[http-equiv="refresh"]')).toBeNull()
      expect(Array.from(parsed.scripts).map(script => script.nonce)).toEqual(['page-nonce', 'page-nonce'])
      expect(parsed.querySelector('script[src],script[href]')).toBeNull()
      expect(parsed.head.firstElementChild?.getAttribute('http-equiv')).toBe('Content-Security-Policy')
      expect(frame.srcdoc).not.toContain('```')
      expect(frame.srcdoc).toContain('window.frame=1')
      expect(raw).toContain('```html')
    } finally {
      nonce.remove()
    }
  })

  it('画布尺寸来自当前作品框架，并且在允许的像素范围内', () => {
    const frame = document.createElement('iframe')
    document.body.appendChild(frame)
    try {
      const source = frame.contentWindow!
      const data = { type: 'pelican-preview:size', token: 'active', width: 1800, height: 900 }
      function read(event: MessageEventInit) {
        return readPelicanCanvas(new MessageEvent('message', { source, origin: 'null', data, ...event }), source, 'active')
      }
      expect(read({})).toEqual({ width: 1800, height: 900 })
      expect(read({ source: window })).toBeNull()
      expect(read({ origin: 'https://fixture.invalid' })).toBeNull()
      expect(read({ data: { ...data, token: 'past' } })).toBeNull()
      expect(read({ data: { ...data, width: 8193 } })).toBeNull()
      expect(read({ data: { ...data, height: Infinity } })).toBeNull()
    } finally {
      frame.remove()
    }
  })

  it.each([
    ['只有 viewBox', 'viewBox="-10, 4, 600, 1200"', 600, 1200],
    ['根尺寸相对视口', 'width="100%" height="100vh" viewBox="0 0 720 400"', 720, 400],
    ['一边使用像素', 'width="320px" height="100%" viewBox="0 0 800 400"', 320, 160],
    ['固定尺寸与 viewBox 不同', 'width="800" height="400" viewBox="0 0 1600 800"', 800, 400],
    ['早期作品无尺寸', '', 1024, 768],
    ['相对尺寸且无 viewBox', 'width="100%" height="100vh"', 1024, 768],
    ['声明尺寸过大', 'width="999999" height="999999"', 1024, 768],
  ])('按原尺寸查看%s的作品', async (_case, attrs, width, height) => {
    mockReads(path => path === '/pelican-tests'
      ? [{ group_id: 2, group_name: 'Group A', models: [{ ...plan, latest: result }] }]
      : { ...detail, answer: `<main><svg ${attrs}><circle r="4"/></svg></main>` })
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()
    await view.get('[data-testid="open-sample-26"]').trigger('click')
    await flushPromises()
    const dialog = view.get('[role="dialog"]')
    await dialog.get('[data-testid="showcase-preview-actual"]').trigger('click')
    const image = dialog.get<HTMLImageElement>('img')
    expect(image.element.style.width).toBe(`${width}px`)
    expect(image.element.style.height).toBe(`${height}px`)
    expect(image.element.style.transform).toBe('scale(1)')
    expect(decodeURIComponent(image.attributes('src'))).not.toContain('<main>')
    expect(dialog.find('iframe, object, svg, script').exists()).toBe(false)
  })

  it('卡片和详情按同一画布缩放，切换原尺寸后可以滚动，下一幅作品默认适应窗口', async () => {
    const viewportWidth = vi.spyOn(Element.prototype, 'clientWidth', 'get').mockReturnValue(300)
    const viewportHeight = vi.spyOn(Element.prototype, 'clientHeight', 'get').mockReturnValue(240)
    vi.stubGlobal('ResizeObserver', undefined)
    try {
      const small = { ...result, id: 25 }
      mockReads(path => {
        if (path === '/pelican-tests') return [{
          group_id: 2, group_name: 'Group A', models: [{ ...plan, latest: result, artworks: [result, small] }],
        }]
        const answer = path.endsWith('/26')
          ? '<svg width="100%" height="100vh" viewBox="0 0 1200 600"><circle r="4"/></svg>'
          : '<svg width="180" height="100"><circle r="2"/></svg>'
        return { ...detail, ...(path.endsWith('/26') ? result : small), answer }
      })
      const view = mount(UserPelicanTestsView, { global })
      await flushPromises()
      const card = view.get('[data-testid="open-sample-26"]')
      await card.trigger('focus')
      await flushPromises()
      const thumbnail = view.get<HTMLImageElement>('[data-testid="artwork-strip"] img').element
      expect(thumbnail.style.width).toBe('1200px')
      expect(thumbnail.style.height).toBe('600px')
      expect(thumbnail.style.transform).toBe('scale(0.25)')
      await card.trigger('click')
      await flushPromises()
      const dialog = view.get('[role="dialog"]')
      const fit = dialog.get('[data-testid="showcase-preview-fit"]')
      const actual = dialog.get('[data-testid="showcase-preview-actual"]')
      const image = dialog.get<HTMLImageElement>('img').element
      expect(fit.attributes('aria-pressed')).toBe('true')
      expect(image.src).toBe(thumbnail.src)
      expect(image.style.transform).toBe('scale(0.25)')
      viewportWidth.mockReturnValue(900)
      viewportHeight.mockReturnValue(225)
      window.dispatchEvent(new Event('resize'))
      await flushPromises()
      expect(image.style.transform).toBe('scale(0.375)')
      await actual.trigger('click')
      expect(actual.attributes('aria-pressed')).toBe('true')
      expect(image.style.transform).toBe('scale(1)')
      expect(dialog.get('[data-testid="pelican-artwork-preview"]').attributes('tabindex')).toBe('0')
      await fit.trigger('click')
      expect(image.style.transform).toBe('scale(0.375)')
      await actual.trigger('click')
      await view.get('[data-testid="open-sample-25"]').trigger('click')
      await flushPromises()
      expect(dialog.get('[data-testid="showcase-preview-fit"]').attributes('aria-pressed')).toBe('true')
      expect(dialog.get<HTMLImageElement>('img').element.style.transform).toBe('scale(1)')
      expect(dialog.get<HTMLImageElement>('img').element.style.width).toBe('180px')
      setPelicanVisibility({ enabled: false, api_enabled: true })
      await flushPromises()
      expect(view.find('[role="dialog"]').exists()).toBe(false)
    } finally {
      viewportWidth.mockRestore()
      viewportHeight.mockRestore()
      vi.unstubAllGlobals()
    }
  })

  it('计划换题后在作答时的绘图题下展示历史作品，只在当前题目列出计划状态', async () => {
    const artwork = { ...result, question_kind: 'custom_svg', question_name: '海边骑车', reasoning_effort: 'low' }
    const candyPlan = { ...plan, question_kind: 'candy', question_name: '分糖果', reasoning_effort: 'high', run_state: 'running' }
    mockReads(path => path === '/pelican-tests'
      ? [{ group_id: 2, group_name: 'Group A', models: [{ ...candyPlan, latest: null, artworks: [artwork] }] }]
      : { ...detail, ...artwork })
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()

    const sections = view.findAll('section').filter(section => section.find('h2').exists())
    expect(sections.map(section => section.get('h2').text())).toEqual([
      '海边骑车pelicanTests.kind.custom_svg', '分糖果pelicanTests.kind.candy',
    ])
    const [drawing, candy] = sections
    const card = drawing.get('[data-testid="open-sample-26"]')
    expect(drawing.get('[data-testid="artwork-strip"]').text()).toContain('pelicanTests.effort.low')
    expect(drawing.get('model-icon-stub').attributes('model')).toBe(artwork.model_id)
    expect(drawing.findAll('ul')).toHaveLength(1)
    expect(drawing.text()).not.toContain('pelicanTests.configuredEffort')
    expect(drawing.text()).not.toContain('pelicanTests.runState.running')
    expect(candy.text()).toContain('pelicanTests.configuredEffort pelicanTests.effort.high')
    expect(candy.text()).toContain('pelicanTests.runState.running')
    expect(candy.find('[data-testid="open-sample-26"]').exists()).toBe(false)
    await card.trigger('click')
    await flushPromises()
    expect(view.get('[role="dialog"]').text()).toContain('海边骑车')
  })

  it('按分组显示探针的状态变化和请求详情', async () => {
    const probe = { ...result, question_kind: 'state_probe', status: 'suspected', detail_code: 'state_changed' }
    mockReads(path => {
      if (path === '/pelican-tests') return [{
        group_id: 2, group_name: 'My group', models: [{ ...plan, question_kind: 'state_probe', latest: probe }],
      }]
      if (path === '/pelican-tests/results/26') return {
        ...probe, prompt: '连续两次请求', criteria: [], answer: '首次请求 HTTP 200\n续接请求 HTTP 200', provider_name: 'Private provider',
      }
      throw Error(path)
    })
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()
    expect(view.text()).toContain('pelicanTests.probe.status.suspected')
    expect(view.text()).not.toContain('pelicanTests.configuredEffort')
    expect(view.find('[data-testid="answer-strip"]').exists()).toBe(false)
    await view.get('[data-testid="open-sample-26"]').trigger('click')
    await flushPromises()
    expect(view.text()).toContain('pelicanTests.probe.observation')
    expect(view.text()).toContain('续接请求 HTTP 200')
    expect(view.text()).not.toContain('Private provider')
    expect(view.text()).not.toContain('pelicanTests.reasoningEffort')
    view.unmount()
  })

  it('清单或样本详情返回 404 时清空已打开的作品和列表，刷新后使用当前权限重新读取', async () => {
    vi.useFakeTimers()
    try {
      let removed = false
      let listGone = false
      mockReads(path => {
        if (path === '/pelican-tests') {
          if (listGone) throw { status: 404 }
          return [{ group_id: 2, group_name: 'Group A', models: [{ ...plan, latest: result }] }]
        }
        if (path.endsWith('/26')) {
          if (removed) throw { status: 404 }
          return detail
        }
        throw Error(path)
      })
      const view = mount(UserPelicanTestsView, { global })
      await flushPromises()
      await view.get('[data-testid="open-sample-26"]').trigger('click')
      await flushPromises()
      expect(view.get('[role="dialog"]').text()).toContain('Draw a pelican')
      removed = true
      await vi.advanceTimersByTimeAsync(30000)
      await flushPromises()
      expect(view.find('[role="dialog"]').exists()).toBe(false)
      expect(view.find('[data-testid="open-sample-26"]').exists()).toBe(false)
      await view.get('[data-testid="refresh-samples"]').trigger('click')
      await flushPromises()
      expect(view.find('[data-testid="open-sample-26"]').exists()).toBe(true)
      listGone = true
      await view.get('[data-testid="refresh-samples"]').trigger('click')
      await flushPromises()
      expect(view.find('[data-testid="open-sample-26"]').exists()).toBe(false)
    } finally {
      vi.useRealTimers()
    }
  })

  it('切换账号后忽略前个账号的迟到读取，关闭展示会移除留在页面的作品和详情', async () => {
    let finish!: (value: unknown) => void
    let markFirst = true
    mockReads(path => {
      if (path.endsWith('/visibility')) return { enabled: true, api_enabled: false }
      if (path.endsWith('/26')) return detail
      if (markFirst) {
        markFirst = false
        return new Promise(resolve => { finish = resolve })
      }
      return [{ group_id: 3, group_name: 'New account', models: [{ ...plan, latest: result }] }]
    })
    await readPelicanVisibility(1, true)
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()
    await readPelicanVisibility(2, true)
    await flushPromises()
    finish([{ group_id: 2, group_name: 'Old account', models: [{ ...plan, latest: result }] }])
    await flushPromises()
    expect(view.text()).toContain('New account')
    expect(view.text()).not.toContain('Old account')
    await view.get('[data-testid="open-sample-26"]').trigger('click')
    await flushPromises()
    expect(view.get('[role="dialog"]').text()).toContain('Draw a pelican')
    setPelicanVisibility({ enabled: false, api_enabled: true })
    await flushPromises()
    expect(view.find('[role="dialog"]').exists()).toBe(false)
    expect(view.find('[data-testid="open-sample-26"]').exists()).toBe(false)
    expect(view.find('[data-testid="public-showcase-api-info"]').exists()).toBe(false)
  })

  it('中英文界面把内置默认名和未命名的历史作品放在同一画廊，自定义题及改名的计划单独展示', async () => {
    const withName = { ...result, id: 32, question_name: '鹈鹕 SVG' }
    const custom = { ...result, id: 36, question_kind: 'custom_svg', question_name: '鹈鹕 SVG' }
    const renamed = { ...result, id: 37, question_name: '我的鹈鹕' }
    mockReads(() => [{
      group_id: 2, group_name: 'Group A', models: [
        { ...plan, latest: result, artworks: [result] },
        { ...plan, question_name: withName.question_name, latest: withName, artworks: [withName] },
        { ...plan, question_kind: 'custom_svg', question_name: custom.question_name, latest: custom, artworks: [custom] },
        { ...plan, question_name: renamed.question_name, latest: renamed, artworks: [renamed] },
      ],
    }])
    const view = mount(UserPelicanTestsView, { global })
    for (const next of ['zh-Hans', 'en']) {
      language.value = next
      await flushPromises()
      const strips = view.findAll('[data-testid="artwork-strip"]')
      expect(strips).toHaveLength(3)
      expect(strips[0].findAll('button').map(el => el.attributes('data-testid'))).toEqual(['open-sample-32', 'open-sample-26'])
      expect(strips[1].get('button').attributes('data-testid')).toBe('open-sample-36')
      expect(strips[2].get('button').attributes('data-testid')).toBe('open-sample-37')
      expect(view.text()).toContain('我的鹈鹕')
    }
  })

  it('同组同模型的绘图题各有画廊，只加载进入视口的作品并缓存预览，文本与失败记录单列', async () => {
    const targets = new Map<Element, (visible: boolean) => void>()
    class Observer {
      constructor(private readonly callback: (entries: { isIntersecting: boolean }[]) => void) {}
      observe(target: Element) { targets.set(target, visible => this.callback([{ isIntersecting: visible }])) }
      disconnect() {}
    }
    vi.stubGlobal('IntersectionObserver', Observer)
    try {
      const first = { ...result, question_kind: 'custom_svg', question_name: '帆船', reasoning_effort: 'xhigh', latency_ms: 214517 }
      const older = { ...first, id: 25, reasoning_effort: 'default', tested_at: '2026-11-17T09:00:00Z' }
      const other = { ...first, id: 23, question_name: '彩虹', reasoning_effort: undefined }
      const failed = { ...first, id: 27, status: 'inconclusive', detail_code: 'upstream_error', latency_ms: 9 }
      const text = { ...result, id: 31, question_kind: 'custom_text', question_name: '拼字', status: 'passed' }
      const group = { group_id: 2, group_name: 'My group', models: [
        { ...plan, question_kind: 'custom_svg', question_name: '帆船', latest: failed, artworks: [first, older] },
        { ...plan, question_kind: 'custom_svg', question_name: '彩虹', latest: other, artworks: [other] },
        { ...plan, question_kind: 'custom_text', question_name: '拼字', latest: text },
      ] }
      mockReads(path => {
        if (path === '/pelican-tests') return [group]
        if (path.endsWith('/26')) return { ...detail, ...first }
        if (path.endsWith('/31')) return { ...detail, ...text, answer: '<b>cat</b>' }
        throw Error(path)
      })
      const view = mount(UserPelicanTestsView, { global })
      await flushPromises()
      const rows = view.findAll('[data-testid="artwork-strip"]')
      expect(rows).toHaveLength(2)
      expect(rows[0].findAll('button').map(b => b.attributes('data-testid'))).toEqual(['open-sample-26', 'open-sample-25'])
      expect(rows[1].get('button').attributes('data-testid')).toBe('open-sample-23')
      expect(rows[0].text()).toContain('pelicanTests.effort.xhigh')
      expect(rows[0].text()).toContain('pelicanTests.effort.default')
      expect(rows[1].text()).toContain('pelicanTests.effort.unknown')
      expect(rows[0].text()).toContain('214.5')
      expect(rows[0].get('time').attributes('datetime')).toBe(first.tested_at)
      expect(view.text()).toContain('帆船')
      expect(view.text()).toContain('彩虹')
      expect(view.text()).toContain('拼字')
      expect(view.get('[data-testid="open-sample-27"]').exists()).toBe(true)
      expect(view.text()).toContain('pelicanTests.durationMillis 9')
      expect(http.get).toHaveBeenCalledTimes(1)
      const card = view.get('[data-testid="open-sample-26"]')
      const target = Array.from(targets.keys()).find(el => el === card.element || el.contains(card.element))!
      targets.get(target)!(true)
      await flushPromises()
      const source = rows[0].get('img').attributes('src')
      expect(source).toMatch(/^data:image\/svg\+xml/)
      expect(decodeURIComponent(source)).not.toContain('<main>')
      await view.get('[data-testid="refresh-samples"]').trigger('click')
      await flushPromises()
      expect(rows[0].get('img').attributes('src')).toBe(source)
      expect(http.get.mock.calls.map(call => call[0])).toEqual(['/pelican-tests', '/pelican-tests/results/26', '/pelican-tests'])
      await view.get('[data-testid="open-sample-31"]').trigger('click')
      await flushPromises()
      expect(view.get('[role="dialog"]').text()).toContain('<b>cat</b>')
      expect(view.find('[role="dialog"] b').exists()).toBe(false)
    } finally {
      vi.unstubAllGlobals()
    }
  })

  it('同题的两个计划按配置强度区分，画廊按时间合并作品并显示实际使用的强度', async () => {
    const low = { ...result, id: 43, latency_ms: 7, reasoning_effort: 'none', tested_at: '2026-11-18T10:10:00Z' }
    const high = { ...result, id: 44, reasoning_effort: 'medium', tested_at: '2026-11-18T10:12:00Z' }
    mockReads(() => [{
      group_id: 2, group_name: 'Group A', models: [
        { ...plan, reasoning_effort: 'low', latest: low, artworks: [low] },
        { ...plan, reasoning_effort: 'xhigh', latest: high, artworks: [high] },
      ],
    }])
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()
    const strip = view.get('[data-testid="artwork-strip"]')
    expect(strip.findAll('button').map(card => card.attributes('data-testid'))).toEqual(['open-sample-44', 'open-sample-43'])
    expect(strip.text()).toContain('pelicanTests.effort.none')
    expect(strip.text()).toContain('pelicanTests.durationMillis 7')
    expect(strip.text()).toContain('pelicanTests.effort.medium')
    expect(view.text()).toContain('pelicanTests.configuredEffort pelicanTests.effort.low')
    expect(view.text()).toContain('pelicanTests.configuredEffort pelicanTests.effort.xhigh')
  })

  it('合并同题同分组的糖果作答，按时间和编号倒序展示正文、模型和实际档位', async () => {
    const first = { ...result, id: 45, question_kind: 'candy', status: 'passed', answer_preview: '<b>21</b>', reasoning_effort: 'default', latency_ms: 7, tested_at: '2026-11-18T09:00:00.123456Z' }
    const wrong = { ...first, id: 46, status: 'suspected', answer_preview: '20 颗', reasoning_effort: 'none', schedule_action: 'disabled' }
    const older = { ...first, id: 60, model_id: 'model-b', answer_preview: '二十一颗糖。\n这个结论来自三个袋子中糖果数量的总和。', reasoning_effort: undefined, tested_at: '2026-11-18T09:00:00.123455Z' }
    const failed = { ...result, id: 51, question_kind: 'candy', status: 'inconclusive', tested_at: '2026-11-18T09:00:01Z' }
    const otherGroup = { ...first, id: 62, answer_preview: 'B 组的答案' }
    const renamed = { ...first, id: 63, question_name: '第二道糖果题', answer_preview: '8 颗' }
    const candyPlan = { ...plan, question_kind: 'candy' }
    mockReads(path => {
      if (path === '/pelican-tests') return [
        { group_id: 1, group_name: 'Group A', models: [
          { ...candyPlan, question_name: '糖果题', reasoning_effort: 'high', latest: failed, answers: [first] },
          { ...candyPlan, reasoning_effort: 'low', latest: wrong, answers: [wrong] },
          { ...candyPlan, model_id: older.model_id, latest: older, answers: [older] },
          { ...candyPlan, question_name: renamed.question_name, latest: renamed, answers: [renamed] },
          { ...plan, question_kind: 'custom_text', latest: null },
        ] },
        { group_id: 2, group_name: 'Group B', models: [{ ...candyPlan, latest: otherGroup, answers: [otherGroup] }] },
      ]
      if (path === '/pelican-tests/results/45') return { ...first, prompt: '袋里有几颗糖果？', criteria: [], answer: '完整解释\n<b>21</b>' }
      throw Error(path)
    })
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()
    const strips = view.findAll('[data-testid="answer-strip"]')
    expect(strips.map(strip => strip.findAll('button').map(card => card.attributes('data-testid')))).toEqual([
      ['open-sample-46', 'open-sample-45', 'open-sample-60'], ['open-sample-62'], ['open-sample-63'],
    ])
    const card = strips[0].get('[data-testid="open-sample-45"]')
    expect(card.text()).toContain('<b>21</b>')
    expect(card.find('b').exists()).toBe(false)
    expect(card.text()).toContain('model-a')
    expect(card.text()).toContain('pelicanTests.effort.default')
    expect(card.text()).toContain('pelicanTests.durationMillis 7')
    expect(card.text()).toContain('pelicanTests.status.passed')
    expect(card.get('time').attributes('datetime')).toBe(first.tested_at)
    expect(strips[0].get('[data-testid="open-sample-46"]').text()).toContain('pelicanTests.status.suspected')
    expect(strips[0].get('[data-testid="open-sample-46"]').text()).toContain('pelicanTests.candySuspension.disabled')
    expect(strips[0].text()).toContain('pelicanTests.effort.none')
    expect(strips[0].text()).toContain('pelicanTests.effort.unknown')
    expect(strips[0].get('[data-testid="open-sample-60"]').text()).toContain(older.answer_preview)
    expect(view.findAll('[data-testid="open-sample-46"]')).toHaveLength(1)
    expect(view.get('[data-testid="open-sample-51"]').exists()).toBe(true)
    expect(view.text()).toContain('pelicanTests.status.inconclusive')
    expect(view.text()).toContain('pelicanTests.paused')
    await card.trigger('focus')
    await flushPromises()
    expect(http.get.mock.calls.map(call => call[0])).toEqual(['/pelican-tests'])
    await card.trigger('click')
    await flushPromises()
    expect(view.get('[role="dialog"]').text()).toContain('袋里有几颗糖果？')
    expect(view.get('[role="dialog"]').text()).toContain('完整解释')
    expect(view.get('[role="dialog"]').find('b').exists()).toBe(false)
    expect(http.get.mock.calls.map(call => call[0])).toEqual(['/pelican-tests', '/pelican-tests/results/45'])
  })

  it('切换账号或关闭展示时清空糖果答案和打开的详情', async () => {
    const candy = { ...result, question_kind: 'candy', status: 'passed' }
    let currentAnswer = '上一个账号的作答'
    let finishDetail!: (value: unknown) => void
    mockReads(path => {
      if (path.endsWith('/visibility')) return { enabled: true, api_enabled: false }
      if (path.endsWith('/26')) return new Promise(resolve => { finishDetail = resolve })
      return [{ group_id: 2, group_name: 'Group A', models: [
        { ...plan, question_kind: 'candy', latest: candy, answers: [{ ...candy, answer_preview: currentAnswer }] },
      ] }]
    })
    await readPelicanVisibility(1, true)
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()
    expect(view.text()).toContain(currentAnswer)
    await view.get('[data-testid="open-sample-26"]').trigger('click')
    await flushPromises()
    currentAnswer = '当前账号的作答'
    await readPelicanVisibility(2, true)
    finishDetail({ ...detail, ...candy, answer: '上一个账号的完整作答' })
    await flushPromises()
    expect(view.text()).toContain(currentAnswer)
    expect(view.text()).not.toContain('上一个账号')
    expect(view.find('[role="dialog"]').exists()).toBe(false)
    setPelicanVisibility({ enabled: false, api_enabled: false })
    await flushPromises()
    expect(view.text()).not.toContain(currentAnswer)
    expect(view.find('[data-testid="answer-strip"]').exists()).toBe(false)
    setPelicanVisibility({ enabled: true, api_enabled: false })
    await flushPromises()
    expect(view.text()).toContain(currentAnswer)
  })

  it('按分组展示最近的样本和未采样模型，并用文本展示糖果答案', async () => {
    const candy = { ...result, question_kind: 'candy', status: 'passed' }
    mockReads(path => {
      if (path === '/pelican-tests') return [{
        group_id: 2, group_name: 'My group', models: [
          { model_id: 'model-a', question_kind: 'candy', enabled: true, run_state: 'idle', latest: candy },
          { model_id: 'model-b', question_kind: 'pelican', enabled: true, run_state: 'queued', next_run_at: '2026-11-19T09:00:00Z', latest: null },
        ],
      }]
      if (path === '/pelican-tests/results/26') return {
        ...candy, prompt: 'Count the candy', criteria: [], answer: '<b>42</b>', provider_name: 'Private provider',
      }
      throw Error(path)
    })
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()
    expect(view.text()).toContain('My group')
    expect(view.text()).toContain('model-b')
    expect(view.text()).toContain('pelicanTests.noSample')
    expect(view.text()).toContain('pelicanTests.runState.queued')
    expect(view.find('[data-testid="answer-strip"]').exists()).toBe(false)
    expect(view.find('time[datetime="2026-11-19T09:00:00Z"]').exists()).toBe(true)
    expect(view.find(`time[datetime="${result.tested_at}"]`).exists()).toBe(true)
    await view.get('[data-testid="open-sample-26"]').trigger('click')
    await flushPromises()
    expect(view.text()).toContain('Count the candy')
    expect(view.text()).toContain('<b>42</b>')
    expect(view.find('b').exists()).toBe(false)
    expect(view.text()).not.toContain('Private provider')
    expect(view.find('[data-testid="review-suspected"]').exists()).toBe(false)
    expect(http.get.mock.calls.map(call => call[0])).toEqual(['/pelican-tests', '/pelican-tests/results/26'])
  })

  it('重新加载失败的用户列表时显示空状态，不显示服务器原始错误', async () => {
    http.get.mockRejectedValueOnce(Error('upstream-internal-host')).mockResolvedValueOnce({ data: [] })
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()
    expect(view.get('[role="alert"]').text()).toContain('pelicanTests.samplesLoadFailed')
    expect(view.text()).not.toContain('upstream-internal-host')
    await view.get('[data-testid="refresh-samples"]').trigger('click')
    await flushPromises()
    expect(view.text()).toContain('pelicanTests.noGroups')
    expect(view.find('[role="alert"]').exists()).toBe(false)
  })

  it('取样前无法选号时显示原因，隐藏空白的题目面板', async () => {
    const sample = { ...result, status: 'inconclusive', detail_code: 'no_provider' }
    mockReads(path => path.endsWith('/26')
      ? { ...sample, answer: '', prompt: '', criteria: [] }
      : [{ group_id: 2, group_name: 'Group A', models: [{ ...plan, latest: sample }] }])
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()
    await view.get('[data-testid="open-sample-26"]').trigger('click')
    await flushPromises()
    expect(view.text()).toContain('pelicanTests.reason.no_provider')
    expect(view.text()).not.toContain('pelicanTests.prompt')
  })

  it('最新采样失败时仍跟进较早绘图的待复核状态，保留作答时实际关闭的推理强度', async () => {
    vi.useFakeTimers()
    try {
      let reviewed = false
      const latest = { ...result, id: 27, status: 'inconclusive', detail_code: 'unsupported_effort' }
      mockReads(path => {
        if (path.endsWith('/27')) return { ...detail, ...latest, answer: '' }
        return [{ group_id: 2, group_name: 'Group A', models: [{
          ...plan, latest, artworks: [{ ...result, reasoning_effort: 'none', status: reviewed ? 'passed' : 'pending_review' }],
        }] }]
      })
      const view = mount(UserPelicanTestsView, { global })
      await flushPromises()
      expect(view.get('[data-testid="artwork-strip"]').text()).toContain('pelicanTests.effort.none')
      await view.get('[data-testid="open-sample-27"]').trigger('click')
      await flushPromises()
      expect(view.get('[role="dialog"]').text()).toContain('pelicanTests.reason.unsupported_effort')
      reviewed = true
      await vi.advanceTimersByTimeAsync(30000)
      await flushPromises()
      expect(view.get('[data-testid="artwork-strip"]').text()).toContain('pelicanTests.status.passed')
      await vi.advanceTimersByTimeAsync(60000)
      expect(http.get.mock.calls.map(call => call[0])).toEqual(['/pelican-tests', '/pelican-tests/results/27', '/pelican-tests'])
    } finally {
      vi.useRealTimers()
    }
  })

  it('切换界面语言时重新读取分组的展示名称', async () => {
    mockReads(() => [{ group_id: 2, group_name: language.value === 'en' ? 'My group' : '我的分组', models: [] }])
    const view = mount(UserPelicanTestsView, { global })
    await flushPromises()
    expect(view.text()).toContain('我的分组')
    language.value = 'en'
    await flushPromises()
    expect(view.text()).toContain('My group')
  })

  it('定时计划空闲时也会发现新样本，离开页面会停止轮询', async () => {
    vi.useFakeTimers()
    try {
      let sampled = false
      mockReads(() => [{
        group_id: 2, group_name: 'Group A',
        models: [{ ...plan, enabled: true, run_state: 'idle', latest: sampled ? result : null }],
      }])
      const view = mount(UserPelicanTestsView, { global })
      await flushPromises()
      expect(view.text()).toContain('pelicanTests.noSample')
      sampled = true
      await vi.advanceTimersByTimeAsync(2500)
      expect(http.get).toHaveBeenCalledTimes(1)
      await vi.advanceTimersByTimeAsync(27500)
      await flushPromises()
      expect(view.text()).toContain('pelicanTests.status.pending_review')
      view.unmount()
      await vi.advanceTimersByTimeAsync(30000)
      expect(http.get).toHaveBeenCalledTimes(2)
    } finally {
      vi.useRealTimers()
    }
  })

  it('停用计划的待复核样本会跟随管理员结论更新列表和详情，定案后停止轮询', async () => {
    vi.useFakeTimers()
    try {
      let status = 'pending_review'
      mockReads(path => {
        if (path === '/pelican-tests') return [{
          group_id: 2, group_name: 'Group A',
          models: [{ ...plan, enabled: false, run_state: 'idle', latest: { ...result, status } }],
        }]
        if (path === '/pelican-tests/results/26') return { ...detail, status }
        throw Error(path)
      })
      const view = mount(UserPelicanTestsView, { global })
      await flushPromises()
      await view.get('[data-testid="open-sample-26"]').trigger('click')
      await flushPromises()
      expect(view.get('li .badge').text()).toBe('pelicanTests.status.pending_review')
      expect(view.get('[role="dialog"] .badge').text()).toBe('pelicanTests.status.pending_review')

      status = 'suspected'
      await vi.advanceTimersByTimeAsync(29999)
      expect(http.get).toHaveBeenCalledTimes(2)
      await vi.advanceTimersByTimeAsync(1)
      await flushPromises()
      expect(view.get('li .badge').text()).toBe('pelicanTests.status.suspected')
      expect(view.get('[role="dialog"] .badge').text()).toBe('pelicanTests.status.suspected')
      expect(http.get.mock.calls.filter(([path]) => path === '/pelican-tests/results/26')).toHaveLength(2)

      await vi.advanceTimersByTimeAsync(60000)
      expect(http.get).toHaveBeenCalledTimes(4)
    } finally {
      vi.useRealTimers()
    }
  })
})
