import { mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import OAuthAuthorizationFlow from '../OAuthAuthorizationFlow.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({
    copied: false,
    copyToClipboard: vi.fn()
  })
}))

describe('OAuthAuthorizationFlow', () => {
  it('emits Codex PAT token for OpenAI PAT auth mode', async () => {
    const wrapper = mount(OAuthAuthorizationFlow, {
      props: {
        addMethod: 'oauth',
        platform: 'openai',
        showCookieOption: false,
        showRefreshTokenOption: false,
        showCodexPatOption: true
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    const patOption = wrapper.find('[data-testid="oauth-method-codex_pat"]')
    expect(patOption.exists()).toBe(true)

    await patOption.trigger('click')
    const tokenInput = wrapper.find('input[type="password"]')
    await tokenInput.setValue(' at-test-token ')
    await wrapper.find('button.btn-primary').trigger('click')

    expect(wrapper.emitted('update:inputMethod')?.at(-1)).toEqual(['codex_pat'])
    expect(wrapper.emitted('import-codex-pat')).toEqual([['at-test-token']])
  })

  it('emits trimmed batch input for Grok SSO auth mode', async () => {
    const wrapper = mount(OAuthAuthorizationFlow, {
      props: {
        addMethod: 'oauth',
        platform: 'grok',
        showSsoOption: true
      },
      global: {
        stubs: {
          Icon: true
        }
      }
    })

    const ssoOption = wrapper.find('[data-testid="oauth-method-sso_cookie"]')
    expect(ssoOption.exists()).toBe(true)

    await ssoOption.trigger('click')
    await wrapper.find('textarea').setValue('  sso-one\nsso-two  ')
    await wrapper.find('button.btn-primary').trigger('click')

    expect(wrapper.emitted('update:inputMethod')?.at(-1)).toEqual(['sso_cookie'])
    expect(wrapper.emitted('import-sso')).toEqual([['sso-one\nsso-two']])
  })

  it('授权方式超过四种时改用下拉框', () => {
    const wrapper = mount(OAuthAuthorizationFlow, {
      props: {
        addMethod: 'oauth',
        platform: 'openai',
        showCookieOption: false,
        showRefreshTokenOption: true,
        showMobileRefreshTokenOption: true,
        showCodexSessionImportOption: true,
        showAgentIdentityOption: true,
        showCodexPatOption: true
      },
      global: { stubs: { Icon: true } }
    })

    expect(wrapper.find('[data-testid="oauth-method-select"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="oauth-method-manual"]').exists()).toBe(false)
  })
})
