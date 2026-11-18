import { useProtocolCatalogFixture } from '@/__tests__/helpers/protocolCatalog'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ProvidersView from '../ProvidersView.vue'

const {
  listProviders,
  listWithEtag,
  getBatchTodayStats,
  exportData,
  getAllProxies,
  getAllGroups,
  showWarning,
  showError
} = vi.hoisted(() => ({
  listProviders: vi.fn(),
  listWithEtag: vi.fn(),
  getBatchTodayStats: vi.fn(),
  exportData: vi.fn(),
  getAllProxies: vi.fn(),
  getAllGroups: vi.fn(),
  showWarning: vi.fn(),
  showError: vi.fn()
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
      getAllIncludingInactive: getAllGroups
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
  emits: ['select-all-results', 'select-page', 'clear'],
  template: `
    <div>
      <span data-test="selected-count">{{ selectedIds.length }}</span>
      <span data-test="total-results">{{ totalResults }}</span>
      <span data-test="all-results-selected">{{ String(allResultsSelected) }}</span>
      <button data-test="select-page" @click="$emit('select-page')">select page</button>
      <button data-test="select-all-results" @click="$emit('select-all-results')">select all</button>
      <button data-test="clear" @click="$emit('clear')">clear</button>
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
})

useProtocolCatalogFixture()
