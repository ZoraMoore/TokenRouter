import { describe, it, expect, vi, beforeEach } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ImportDataModal from '@/components/admin/account/ImportDataModal.vue'
import { adminAPI } from '@/api/admin'
import zhMessages from '@/i18n/locales/zh'

const showError = vi.fn()
const showSuccess = vi.fn()

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: {
      importData: vi.fn()
    },
    settings: {
      getOpenAIOAuthImportDefaults: vi.fn(),
      updateOpenAIOAuthImportDefaults: vi.fn()
    }
  }
}))

vi.mock('@/api/admin/accounts', () => ({
  getAntigravityDefaultModelMapping: vi.fn()
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

describe('ImportDataModal', () => {
  const importData = vi.mocked(adminAPI.accounts.importData)

  beforeEach(() => {
    showError.mockReset()
    showSuccess.mockReset()
    importData.mockReset()
  })

  const mountModal = () => {
    return mount(ImportDataModal, {
      props: { show: true },
      global: {
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })
  }

  const successResult = {
    proxy_created: 0,
    proxy_reused: 0,
    proxy_failed: 0,
    account_created: 1,
    account_failed: 0,
    errors: []
  }

  it('未提供导入来源时提示错误', async () => {
    const wrapper = mountModal()

    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportSelectSource')
    expect(importData).not.toHaveBeenCalled()
  })

  it('粘贴无效 JSON 时提示解析失败', async () => {
    const wrapper = mountModal()

    await wrapper.find('textarea').setValue('invalid json')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportParseFailed')
    expect(importData).not.toHaveBeenCalled()
  })

  it('选择文件中的无效 JSON 时提示解析失败', async () => {
    const wrapper = mountModal()

    const input = wrapper.find('input[type="file"]')
    const file = new File(['invalid json'], 'data.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve('invalid json')
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(showError).toHaveBeenCalledWith('admin.accounts.dataImportParseFailed')
    expect(importData).not.toHaveBeenCalled()
  })

  it('粘贴有效 JSON 时提交解析后的数据', async () => {
    const wrapper = mountModal()
    const payload = { accounts: [{ name: 'pasted-account' }], proxies: [] }
    importData.mockResolvedValue(successResult)

    await wrapper.find('textarea').setValue(JSON.stringify(payload))
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledWith({
      data: payload,
      skip_default_group_bind: true
    })
    expect(showSuccess).toHaveBeenCalledWith('admin.accounts.dataImportSuccess')
  })

  it('同时存在文件和粘贴内容时优先使用粘贴内容', async () => {
    const wrapper = mountModal()
    const filePayload = { accounts: [{ name: 'file-account' }], proxies: [] }
    const pastedPayload = { accounts: [{ name: 'pasted-account' }], proxies: [] }
    importData.mockResolvedValue(successResult)

    const input = wrapper.find('input[type="file"]')
    const file = new File([JSON.stringify(filePayload)], 'data.json', { type: 'application/json' })
    Object.defineProperty(file, 'text', {
      value: () => Promise.resolve(JSON.stringify(filePayload))
    })
    Object.defineProperty(input.element, 'files', {
      value: [file]
    })

    await input.trigger('change')
    await wrapper.find('textarea').setValue(JSON.stringify(pastedPayload))
    await wrapper.find('form').trigger('submit')
    await flushPromises()

    expect(importData).toHaveBeenCalledWith({
      data: pastedPayload,
      skip_default_group_bind: true
    })
  })

  it('粘贴输入框的 JSON 占位符不经过 i18n 消息编译', async () => {
    const actualI18n = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
    const i18n = actualI18n.createI18n({
      legacy: false,
      locale: 'zh',
      messages: {
        zh: zhMessages
      }
    })

    const wrapper = mount(ImportDataModal, {
      props: { show: true },
      global: {
        plugins: [i18n],
        stubs: {
          BaseDialog: { template: '<div><slot /><slot name="footer" /></div>' }
        }
      }
    })

    expect(wrapper.find('textarea').attributes('placeholder')).toContain('"accounts"')
  })
})
