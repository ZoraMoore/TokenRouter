import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import HeaderContactSupport from '../HeaderContactSupport.vue'

vi.mock('@/stores', () => ({ useAppStore: () => ({ contactInfo: '客服：https://example.com/support' }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key === 'common.contactSupport' ? '联系客服' : key }) }))

describe('顶栏客服文字', () => {
  it('展示文字并保留联系方式弹层', async () => {
    const wrapper = mount(HeaderContactSupport, {
      props: { showLabel: true },
      global: { stubs: { Icon: true, MotionTransition: { template: '<div><slot /></div>' } } },
    })
    const button = wrapper.get('[data-testid="contact-support-toggle"]')
    expect(button.text()).toBe('联系客服')
    await button.trigger('click')
    expect(button.attributes('aria-expanded')).toBe('true')
    expect(wrapper.get('a').attributes('href')).toBe('https://example.com/support')
    wrapper.unmount()
  })
})
