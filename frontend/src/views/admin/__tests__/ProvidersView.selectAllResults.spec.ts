import { useProtocolCatalogFixture } from '@/__tests__/helpers/protocolCatalog'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import type { IQStateProbeResult } from '@/api/admin/providerIQ'

import ProvidersView from '../ProvidersView.vue'

const {
  listProviders,
  listWithEtag,
  getBatchTodayStats,
  exportData,
  getAllProxies,
  getAllGroups,
  showWarning,
  showError,
  runIQStateProbe
} = vi.hoisted(() => ({
  listProviders: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  exportData: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  showWarning: vi.fn(),
  showError: vi.fn(),
  runIQStateProbe: vi.fn()
}))

vi.mock('@/api/admin/providerIQ', async original => ({
  ...await original<typeof import('@/api/admin/providerIQ')>(),
  runIQStateProbe
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    providers: {
      list: listProviders,
      listWithEtag,
      getBatchTodayStats,
      exportData,
      batchDelete: vi.fn(),
      batchClearError: vi.fn(),
      batchRefresh: vi.fn(),
      bulkUpdate: vi.fn()
    },
    proxies: {
      getAll: getAllProxies
    },
    groups: {
      getOptions: getAllGroups
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showWarning,
    showSuccess: vi.fn(),
    showInfo: vi.fn()
  })
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => ({
    token: 'test-token'
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

const makeProviders = (count: number) => Array.from({ length: count }, (_, index) => ({
  id: index + 1,
  name: `provider-${index + 1}`,
  platform: 'grok',
  type: 'oauth',
  status: 'active',
  schedulable: true,
  created_at: '2026-07-23T00:00:00Z',
  updated_at: '2026-07-23T00:00:00Z'
}))

const ProviderBulkActionsBarStub = {
  props: ['selectedIds', 'totalResults', 'selectingAll', 'allResultsSelected'],
  emits: ['select-all-results', 'select-page', 'clear', 'run-bulk-probe'],
  template: `
    <div>
      <span data-test="selected-count">{{ selectedIds.length }}</span>
      <span data-test="total-results">{{ totalResults }}</span>
      <span data-test="all-results-selected">{{ String(allResultsSelected) }}</span>
      <button data-test="select-page" @click="$emit('select-page')">select page</button>
      <button data-test="select-all-results" @click="$emit('select-all-results')">select all</button>
      <button data-test="clear" @click="$emit('clear')">clear</button>
      <button data-test="run-bulk-probe" @click="$emit('run-bulk-probe')">run probe</button>
    </div>
  `
}

const ProviderTableFiltersStub = {
  emits: ['change'],
  template: '<button data-test="change-filter" @click="$emit(\'change\')">change filter</button>'
}

const mountView = () => mount(ProvidersView, {
  global: {
    stubs: {
      AppLayout: { template: '<div><slot /></div>' },
      TablePageLayout: {
        template: '<div><slot name="filters" /><slot name="table" /><slot name="pagination" /></div>'
      },
      DataTable: { props: ['data'], template: '<div data-test="data-table"></div>' },
      Pagination: true,
      ConfirmDialog: true,
      BaseDialog: { template: '<section><slot /><slot name="footer" /></section>' },
      TotpStepUpDialog: true,
      ProviderTableActions: { template: '<div><slot name="beforeCreate" /><slot name="after" /></div>' },
      ProviderTableFilters: ProviderTableFiltersStub,
      ProviderBulkActionsBar: ProviderBulkActionsBarStub,
      ProviderActionMenu: true,
      ImportDataModal: true,
      ReAuthProviderModal: true,
      ProviderTestModal: true,
      ProviderStatsModal: true,
      ScheduledTestsPanel: true,
      SyncFromCrsModal: true,
      TempUnschedStatusModal: true,
      ErrorPassthroughRulesModal: true,
      TLSFingerprintProfilesModal: true,
      TLSFingerprintRoutersModal: true,
      CreateProviderModal: true,
      EditProviderModal: true,
      BulkEditProviderModal: true,
      PlatformTypeBadge: true,
      ProviderCapacityCell: true,
      ProviderStatusIndicator: true,
      ProviderTodayStatsCell: true,
      ProviderGroupsCell: true,
      ProviderUsageCell: true,
      Icon: true
    }
  }
})

function probeResult(id: number, verdict: IQStateProbeResult['verdict'] = 'healthy'): IQStateProbeResult {
  return {
    account_id: id,
    requested_model: 'gpt-6-astra',
    model: 'gpt-6-astra',
    verdict,
    mint_status: 200,
    continue_status: verdict === 'inconclusive' ? 429 : 200,
    mint_completed: true,
    continue_completed: verdict !== 'inconclusive',
    mint_latency_ms: 18,
    continue_latency_ms: 24,
    new_ticket: verdict === 'degraded',
    finished_at: '2026-11-19T00:00:00Z',
    latency_ms: 42
  }
}

function pendingProbe() {
  let resolve!: (result: IQStateProbeResult) => void
  const promise = new Promise<IQStateProbeResult>(done => { resolve = done })
  return { promise, resolve }
}

describe('admin ProvidersView select all filtered results', () => {
  beforeEach(() => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    localStorage.clear()
    listProviders.mockReset()
    listWithEtag.mockReset()
    getBatchTodayStats.mockReset()
    exportData.mockReset()
    getAllProxies.mockReset()
    getAllGroups.mockReset()
    showError.mockReset()
    showWarning.mockReset()
    runIQStateProbe.mockReset()

    listWithEtag.mockResolvedValue({
      notModified: true,
      etag: null,
      data: null
    })
    getBatchTodayStats.mockResolvedValue({ stats: {} })
    getAllProxies.mockResolvedValue([])
    getAllGroups.mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.unstubAllGlobals()
  })

  it.each([true, false])('二次验证通过后下载 Sub2API 账号文件，选中本页为 %s', async (selected) => {
    const rows = makeProviders(2)
    listProviders.mockResolvedValue({ items: rows, total: 2, page: 1, page_size: 20, pages: 1 })
    const accounts = [
      { name: 'fixture OAuth', type: 'oauth' },
      { name: 'fixture Qoder', type: 'cosy' }
    ]
    const archive = { exported_at: '2026-11-19T00:00:00Z', accounts, proxies: [], ...(selected ? {} : { skipped_shadows: 1 }) }
    exportData.mockRejectedValueOnce({ reason: 'STEP_UP_REQUIRED' }).mockResolvedValueOnce(archive)
    const createURL = vi.fn((_blob: Blob) => 'blob:fixture')
    const revokeURL = vi.fn()
    vi.stubGlobal('URL', class extends URL {
      static createObjectURL = createURL
      static revokeObjectURL = revokeURL
    })
    const download = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const wrapper = mountView()
    await flushPromises()
    if (selected) await wrapper.get('[data-test="select-page"]').trigger('click')

    const dialog = wrapper.findAllComponents({ name: 'ConfirmDialog' }).find((item) => item.props('title') === 'admin.providers.dataExport')!
    dialog.vm.$emit('confirm')
    await flushPromises()
    expect(download).not.toHaveBeenCalled()
    const { controller } = wrapper.getComponent({ name: 'TotpStepUpDialog' }).props()
    controller.onVerified()
    await flushPromises()

    const scope = selected ? { ids: [1, 2] } : { filters: expect.objectContaining({ sort_by: expect.any(String) }) }
    expect(exportData).toHaveBeenLastCalledWith({ format: 'sub2api', includeProxies: true, ...scope })
    expect(download).toHaveBeenCalledTimes(1)
    expect(download.mock.contexts[0].download).toMatch(/^sub2api-account-\d{14}\.json$/)
    const reader = new FileReader()
    const content = new Promise((resolve) => { reader.onload = () => resolve(reader.result) })
    reader.readAsText(createURL.mock.calls[0][0])
    expect(JSON.parse(String(await content))).toEqual(archive)
    expect(revokeURL).toHaveBeenCalledWith('blob:fixture')
    const warning = selected ? 'admin.providers.dataExportedUnsupportedTypes' : 'admin.providers.dataExportedSkippedShadows admin.providers.dataExportedUnsupportedTypes'
    expect(showWarning).toHaveBeenCalledWith(warning)
    wrapper.unmount()
  })

  it('服务器返回 TokenRouter 文件时提示格式错误并取消下载', async () => {
    listProviders.mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20, pages: 1 })
    exportData.mockResolvedValue({ type: 'tokenrouter-data', version: 2, providers: [], proxies: [] })
    const download = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {})
    const wrapper = mountView()
    await flushPromises()
    const dialog = wrapper.findAllComponents({ name: 'ConfirmDialog' }).find((item) => item.props('title') === 'admin.providers.dataExport')!
    dialog.vm.$emit('confirm')
    await flushPromises()
    expect(showError).toHaveBeenCalledWith('admin.providers.dataExportUnexpectedFormat')
    expect(download).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('selects all matching IDs in one commit and clears the selection when filters change', async () => {
    const allProviders = makeProviders(45)
    listProviders.mockImplementation(async (_page: number, pageSize: number) => {
      if (pageSize === 1000) {
        return {
          items: allProviders,
          total: 45,
          page: 1,
          page_size: 1000,
          pages: 1
        }
      }
      return {
        items: allProviders.slice(0, 20),
        total: 45,
        page: 1,
        page_size: 20,
        pages: 3
      }
    })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="select-all-results"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="selected-count"]').text()).toBe('45')
    expect(wrapper.get('[data-test="total-results"]').text()).toBe('45')
    expect(wrapper.get('[data-test="all-results-selected"]').text()).toBe('true')
    expect(listProviders).toHaveBeenCalledWith(1, 1000, expect.objectContaining({
      lite: '1',
      include_scheduler_score: '0'
    }))

    const vm = wrapper.vm as unknown as { pagination: { total: number } }
    vm.pagination.total = 46
    await wrapper.vm.$nextTick()
    expect(wrapper.get('[data-test="all-results-selected"]').text()).toBe('false')

    await wrapper.get('[data-test="change-filter"]').trigger('click')

    expect(wrapper.get('[data-test="selected-count"]').text()).toBe('0')
    expect(wrapper.get('[data-test="all-results-selected"]').text()).toBe('false')
  })

  it('keeps the original page selection when loading all results fails', async () => {
    const currentPage = makeProviders(20)
    listProviders.mockImplementation(async (_page: number, pageSize: number) => {
      if (pageSize === 1000) {
        throw new Error('load all failed')
      }
      return {
        items: currentPage,
        total: 45,
        page: 1,
        page_size: 20,
        pages: 3
      }
    })

    const wrapper = mountView()
    await flushPromises()

    await wrapper.get('[data-test="select-page"]').trigger('click')
    expect(wrapper.get('[data-test="selected-count"]').text()).toBe('20')

    await wrapper.get('[data-test="select-all-results"]').trigger('click')
    await flushPromises()

    expect(wrapper.get('[data-test="selected-count"]').text()).toBe('20')
    expect(wrapper.get('[data-test="all-results-selected"]').text()).toBe('false')
    expect(showError).toHaveBeenCalledWith('admin.providers.bulkActions.selectAllFailed')
  })

  it('批量探针使用跨页选择快照，按三个并发处理每个账号并显示各自结果', async () => {
    const allProviders = makeProviders(7)
    listProviders.mockImplementation(async (_page: number, pageSize: number) => ({
      items: pageSize === 1000 ? allProviders : allProviders.slice(0, 3), total: 7
    }))
    const pending = allProviders.slice(0, 3).map(pendingProbe)
    runIQStateProbe
      .mockReturnValueOnce(pending[0].promise)
      .mockReturnValueOnce(pending[1].promise)
      .mockReturnValueOnce(pending[2].promise)
      .mockRejectedValueOnce({ reason: 'IQ_TEST_UNSUPPORTED' })
      .mockRejectedValueOnce({ reason: 'IQ_TEST_BUSY' })
      .mockRejectedValueOnce({ reason: 'IQ_TEST_NOT_FOUND' })
      .mockRejectedValueOnce({ reason: 'IQ_TEST_FAILED' })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="select-all-results"]').trigger('click')
    await flushPromises()
    await wrapper.get('[data-test="run-bulk-probe"]').trigger('click')
    expect(runIQStateProbe).toHaveBeenCalledTimes(3)
    expect(runIQStateProbe).toHaveBeenLastCalledWith(3, { model_id: 'gpt-6-astra' }, expect.any(AbortSignal))

    await wrapper.get('[data-test="run-bulk-probe"]').trigger('click')
    await wrapper.get('[data-test="clear"]').trigger('click')
    expect(runIQStateProbe).toHaveBeenCalledTimes(3)

    pending[0].resolve(probeResult(1))
    pending[1].resolve(probeResult(2, 'degraded'))
    pending[2].resolve(probeResult(3, 'inconclusive'))
    await flushPromises()
    expect(runIQStateProbe.mock.calls.map(([id]) => id)).toEqual([1, 2, 3, 4, 5, 6, 7])
    const row = (id: number) => wrapper.get(`[data-testid="provider-bulk-probe-row-${id}"]`)
    expect(row(1).text()).toContain('provider-1')
    expect(row(1).text()).toContain('pelicanTests.probe.status.passed')
    expect(row(2).text()).toContain('pelicanTests.probe.status.suspected')
    expect(row(3).text()).toContain('pelicanTests.probe.status.inconclusive')
    expect(row(4).text()).toContain('#4')
    expect(row(4).text()).toContain('admin.providers.bulkProbe.state.unsupported')
    expect(row(5).text()).toContain('admin.providers.bulkProbe.state.busy')
    expect(row(6).text()).toContain('admin.providers.bulkProbe.state.missing')
    expect(row(7).text()).toContain('admin.providers.bulkProbe.state.failed')
    expect(wrapper.get('[role="progressbar"]').attributes('aria-valuenow')).toBe('7')
    await row(2).get('button').trigger('click')
    expect(row(2).get('[data-testid="iq-probe-next-status"]').text()).toBe('200')
    expect(allProviders.every(provider => provider.schedulable)).toBe(true)
    wrapper.unmount()
  })

  it('停止探针后保留已完成结果，忽略迟到的响应和剩余队列', async () => {
    listProviders.mockResolvedValue({ items: makeProviders(5), total: 5, pages: 1 })
    const pending = pendingProbe()
    runIQStateProbe.mockResolvedValueOnce(probeResult(1)).mockReturnValue(pending.promise)
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="select-page"]').trigger('click')
    await wrapper.get('[data-test="run-bulk-probe"]').trigger('click')
    await flushPromises()
    expect(runIQStateProbe).toHaveBeenCalledTimes(4)

    await wrapper.get('[data-testid="provider-bulk-probe-stop"]').trigger('click')
    expect(runIQStateProbe.mock.calls.every(([, , signal]) => signal.aborted)).toBe(true)
    pending.resolve(probeResult(2, 'degraded'))
    await flushPromises()
    expect(runIQStateProbe).toHaveBeenCalledTimes(4)
    expect(wrapper.get('[data-testid="provider-bulk-probe-row-1"]').text()).toContain('pelicanTests.probe.status.passed')
    for (const id of [2, 3, 4, 5]) {
      expect(wrapper.get(`[data-testid="provider-bulk-probe-row-${id}"]`).text()).toContain('admin.providers.bulkProbe.state.cancelled')
    }
    wrapper.unmount()
  })

  it('全部账号不支持探针时显示每个账号的原因并结束测试', async () => {
    listProviders.mockResolvedValue({ items: makeProviders(4), total: 4, pages: 1 })
    runIQStateProbe.mockRejectedValue({ reason: 'IQ_TEST_UNSUPPORTED' })
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="select-page"]').trigger('click')
    await wrapper.get('[data-test="run-bulk-probe"]').trigger('click')
    await flushPromises()

    expect(runIQStateProbe.mock.calls.map(([id]) => id)).toEqual([1, 2, 3, 4])
    const rows = wrapper.findAll('[data-testid^="provider-bulk-probe-row-"]')
    expect(rows).toHaveLength(4)
    expect(rows.every(row => row.text().includes('admin.providers.bulkProbe.state.unsupported'))).toBe(true)
    expect(wrapper.find('[data-testid="provider-bulk-probe-stop"]').exists()).toBe(false)
    wrapper.unmount()
  })

  it('离开提供商页面时取消在途探针和剩余队列', async () => {
    listProviders.mockResolvedValue({ items: makeProviders(4), total: 4, pages: 1 })
    const pending = pendingProbe()
    runIQStateProbe.mockReturnValue(pending.promise)
    const wrapper = mountView()
    await flushPromises()
    await wrapper.get('[data-test="select-page"]').trigger('click')
    await wrapper.get('[data-test="run-bulk-probe"]').trigger('click')
    expect(runIQStateProbe).toHaveBeenCalledTimes(3)

    wrapper.unmount()
    expect(runIQStateProbe.mock.calls.every(([, , signal]) => signal.aborted)).toBe(true)
    pending.resolve(probeResult(1))
    await flushPromises()
    expect(runIQStateProbe).toHaveBeenCalledTimes(3)
  })
})

useProtocolCatalogFixture()
