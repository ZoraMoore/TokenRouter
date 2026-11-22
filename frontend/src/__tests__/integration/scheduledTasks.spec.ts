import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import pelicanTests from '@/i18n/locales/zh/pelicanTests'
import ScheduledTasksView from '@/views/admin/ScheduledTasksView.vue'
import scheduledTasks from '@/i18n/locales/zh/scheduledTasks'

const http = vi.hoisted(() => ({ get: vi.fn(), post: vi.fn(), put: vi.fn() }))
vi.mock('@/api/client', () => ({ apiClient: http }))
// Vitest 的运行时别名不含消息编译器，这里用完整包验证实际中文插值。
vi.mock('vue-i18n', () => import('../../../node_modules/vue-i18n/dist/vue-i18n.mjs'))

const base = '/admin/scheduled-tasks'
const plan = {
  id: 4, name: '授权巡检', kind: 'relogin', scope: 'all', group_ids: [], provider_ids: [],
  model: '', cron_expression: '*/15 * * * *', enabled: false,
}
const run = {
  id: 27, plan_id: 4, status: 'running', trigger: 'manual', scheduled_at: '2026-11-21T00:00:00Z',
  total: 3, completed: 1, running: 1, remaining: 1,
}

function mountView() {
  return mount(ScheduledTasksView, {
    global: {
      plugins: [createI18n({
        legacy: false, locale: 'zh', messages: { zh: { pelicanTests, scheduledTasks, common: {
          save: '保存', cancel: '取消', close: '关闭', retry: '重试', edit: '编辑', preview: '预览',
          refresh: '刷新', search: '搜索', processing: '处理中', selectedCount: '已选 {count} 项',
        } } }, missingWarn: false, fallbackWarn: false,
      })],
      stubs: {
        AppLayout: { template: '<main><slot name="page-heading-meta"/><slot name="page-heading-actions"/><slot/></main>' },
        BaseDialog: { props: ['show', 'title'], template: '<section v-if="show" role="dialog"><h2>{{ title }}</h2><slot/><slot name="footer"/></section>' },
        Icon: true,
        Select: {
          props: ['modelValue', 'options', 'id', 'disabled'],
          template: `<select :id="id" :value="modelValue" :disabled="disabled" @change="$emit('update:modelValue', $event.target.value)">
            <option v-for="option in options" :key="option.value" :value="option.value">{{ option.label }}</option></select>`,
        },
        Pagination: {
          props: ['page'],
          template: '<button data-testid="next-provider-page" @click="$emit(\'update:page\', page + 1)">Next</button>',
        },
      },
    },
  })
}

enableAutoUnmount(afterEach)

beforeEach(() => {
  vi.resetAllMocks()
})

afterEach(() => {
  vi.useRealTimers()
})

describe('智能运维定时任务', () => {
  it('创建逐账号测智计划，翻页保留账号并按本轮覆盖人数估计请求量', async () => {
    http.get.mockImplementation(async (path: string, options: { params?: { page: number } }) => {
      if (path === `${base}/plans`) return { data: { items: [], timezone: 'Asia/Shanghai' } }
      if (path === '/admin/groups/options') return { data: [] }
      if (path === '/admin/providers') {
        const id = options.params?.page === 2 ? 202 : 101
        return { data: { items: [{ id, name: `账号 ${id}` }], total: 21 } }
      }
      if (path === '/admin/providers/101/models') return { data: [{ id: 'gpt-6-astra' }] }
      throw new Error(path)
    })
    http.post.mockImplementation(async (path: string, draft: object) => {
      if (path === `${base}/preview`) return { data: {
        total: 2,
        eligible: 2,
        items: [101, 202].map(id => ({ provider_id: id, provider_name: `账号 ${id}`, model: 'gpt-6-astra', actual_model: 'gpt-6-astra-upstream' })),
      } }
      if (path === `${base}/plans`) return { data: { ...draft, id: 12 } }
      throw new Error(path)
    })

    const view = mountView()
    await flushPromises()
    await view.get('[data-testid="scheduled-create"]').trigger('click')
    await view.get('#scheduled-plan-name').setValue('每日测智')
    await view.get('#scheduled-plan-kind').setValue('iq')
    expect(view.get<HTMLSelectElement>('#scheduled-plan-frequency').element.value).toBe('every6h')
    expect(view.find('#scheduled-plan-scope option[value="all"]').exists()).toBe(false)
    await view.get('#scheduled-plan-scope').setValue('providers')
    await flushPromises()
    await view.get('[data-testid="scheduled-provider-101"]').setValue(true)
    await view.get('[data-testid="next-provider-page"]').trigger('click')
    await flushPromises()
    await view.get('[data-testid="scheduled-provider-202"]').setValue(true)
    await view.get('#scheduled-plan-model').setValue('gpt-6-astra')
    await view.get('[data-testid="scheduled-preview"]').trigger('click')
    await flushPromises()
    expect(view.text()).toContain('预计发出 4–6 次上游请求')
    expect(view.text()).toContain('gpt-6-astra-upstream')

    await view.get('#scheduled-plan-form').trigger('submit')
    expect(http.post.mock.calls.find(([path]) => path === `${base}/plans`)?.[1]).toEqual({
      name: '每日测智', kind: 'iq', scope: 'providers', provider_ids: [101, 202], group_ids: [],
      model: 'gpt-6-astra', cron_expression: '0 */6 * * *', enabled: false,
    })
    await flushPromises()
  })

  it('立即运行已暂停的计划并显示服务端的重登进度和逐账号结果', async () => {
    vi.useFakeTimers()
    let finished = false
    http.get.mockImplementation(async (path: string) => {
      if (path === `${base}/plans`) return { data: { items: [plan], timezone: 'Asia/Shanghai' } }
      if (path === `${base}/plans/4/runs`) return { data: { items: [run] } }
      if (path === `${base}/runs/27` && finished) return { data: {
        ...run, status: 'finished', completed: 3, running: 0, remaining: 0, items: [
          { provider_id: 1, provider_name: '账号 A', state: 'finished', outcome: 'auth_recovered' },
          { provider_id: 2, provider_name: '账号 B', state: 'finished', outcome: 'relogin_succeeded', replacement_id: 20 },
          { provider_id: 3, provider_name: '账号 C', state: 'finished', outcome: 'needs_action', detail: 'replacement_401' },
        ],
      } }
      if (path === `${base}/runs/27`) return { data: { ...run, items: [
        { provider_id: 1, provider_name: '账号 A', state: 'finished', outcome: 'auth_recovered' },
        { provider_id: 2, provider_name: '账号 B', state: 'queued', stage: 'relogin', operation_id: 'rev-27' },
        { provider_id: 3, provider_name: '账号 C', state: 'queued' },
      ] } }
      throw new Error(path)
    })
    http.post.mockResolvedValue({ data: { run_id: 27 } })

    const view = mountView()
    await flushPromises()
    expect(view.get('main').text()).toContain('Asia/Shanghai')
    await view.get('[data-testid="scheduled-run-4"]').trigger('click')
    await flushPromises()
    const dialog = view.get('[role="dialog"]')
    expect(http.post.mock.calls[0][0]).toBe(`${base}/plans/4/run`)
    expect(dialog.get('progress').attributes('value')).toBe('1')
    expect(dialog.get('progress').attributes('max')).toBe('3')
    expect(dialog.text()).toContain('剩余 1')
    expect(dialog.text()).toContain('已自行恢复')
    expect(dialog.text()).toContain('重登中')
    expect(dialog.text()).toContain('rev-27')

    finished = true
    await vi.advanceTimersByTimeAsync(2500)
    await flushPromises()
    expect(dialog.get('progress').attributes('value')).toBe('3')
    expect(dialog.text()).toContain('剩余 0')
    expect(dialog.text()).toContain('重登后为账号 #20')
    expect(dialog.text()).toContain('重登后的账号仍返回上游 401')

    await vi.advanceTimersByTimeAsync(2500)
    expect(http.get.mock.calls.filter(([path]) => path === `${base}/runs/27`)).toHaveLength(2)
  })

  it('在运行详情中分别显示探针和糖果复测的 HTTP 状态', async () => {
    const iqPlan = { ...plan, kind: 'iq', name: '逐账号测智', last_run_id: 27, last_run_status: 'finished' }
    const finished = { ...run, status: 'finished', total: 1, completed: 1, running: 0, remaining: 0 }
    http.get.mockImplementation(async (path: string) => {
      if (path === `${base}/plans`) return { data: { items: [iqPlan], timezone: 'Asia/Shanghai' } }
      if (path === `${base}/plans/4/runs`) return { data: { items: [finished] } }
      if (path === `${base}/runs/27`) return { data: { ...finished, items: [{
        provider_id: 8, provider_name: '待复测账号', state: 'finished', stage: 'candy', outcome: 'suspended',
        answer: '18', http_status: 200, probe: { verdict: 'degraded', mint_status: 200, continue_status: 200 },
      }] } }
      throw new Error(path)
    })
    const view = mountView()
    await flushPromises()
    await view.get('button.badge').trigger('click')
    await flushPromises()
    const dialog = view.get('[role="dialog"]')
    expect(dialog.text()).toContain('已停调')
    const evidence = dialog.get('details').text().replace(/\s+/g, ' ')
    expect(evidence).toContain('糖果题复测 · 上游 HTTP 200')
    expect(evidence).toContain('首发 HTTP 200 · 续接 HTTP 200')
  })
})
