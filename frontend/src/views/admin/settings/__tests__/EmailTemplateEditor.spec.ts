import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import EmailTemplateEditor from '../EmailTemplateEditor.vue'

const { settings, showError } = vi.hoisted(() => ({
  settings: {
    getEmailTemplates: vi.fn(),
    getEmailTemplate: vi.fn(),
    previewEmailTemplate: vi.fn()
  },
  showError: vi.fn()
}))

vi.mock('@/api', () => ({ adminAPI: { settings } }))
vi.mock('@/stores', () => ({ useAppStore: () => ({ showError, showSuccess: vi.fn() }) }))

beforeEach(() => {
  vi.clearAllMocks()
  settings.getEmailTemplates.mockResolvedValue({
    events: [{ value: 'content_moderation.account_disabled', label: 'Server fallback', category: 'risk_control' }],
    locales: ['zh', 'en'],
    placeholders: []
  })
  settings.getEmailTemplate.mockResolvedValue({ subject: 'Subject', html: '<p>Body</p>', placeholders: [] })
  settings.previewEmailTemplate.mockResolvedValue({ subject: 'Subject', html: '<p>Body</p>' })
})

// 用户封禁事件保留账号语义，后端事件名必须能命中本地化说明。
describe('邮件事件领域边界', () => {
  it.each([
    ['zh', '内容审计禁用账号', '自动禁用用户账号'],
    ['en', 'Risk Control Account Disabled', 'automatically disables the user account']
  ])('使用 %s 文案展示后端的用户封禁事件', async (locale, label, description) => {
    const i18n = createI18n({ legacy: false, locale, messages: {}, missingWarn: false, fallbackWarn: false })
    const wrapper = mount(EmailTemplateEditor, { global: { plugins: [i18n], stubs: { Select: true } } })
    await flushPromises()
    expect(wrapper.text()).toContain(label)
    expect(wrapper.text()).toContain(description)
    expect(wrapper.text()).not.toContain('Server fallback')
    expect(settings.getEmailTemplate).toHaveBeenCalledWith('content_moderation.account_disabled', locale)
    expect(showError).not.toHaveBeenCalled()
    wrapper.unmount()
  })
})
