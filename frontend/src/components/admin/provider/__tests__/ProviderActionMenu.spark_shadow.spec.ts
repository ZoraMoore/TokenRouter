import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import ProviderActionMenu from '../ProviderActionMenu.vue'
import type { Provider } from '@/types'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key,
    }),
  }
})

function makeProvider(overrides: Partial<Provider>): Provider {
  return {
    id: 1,
    name: 'test-provider',
    platform: 'openai',
    type: 'oauth',
    proxy_id: null,
    concurrency: 3,
    priority: 50,
    status: 'active',
    error_message: null,
    last_used_at: null,
    expires_at: null,
    auto_pause_on_expired: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    schedulable: true,
    rate_limited_at: null,
    rate_limit_reset_at: null,
    overload_until: null,
    temp_unschedulable_until: null,
    temp_unschedulable_reason: null,
    session_window_start: null,
    session_window_end: null,
    session_window_status: null,
    ...overrides,
  }
}

const position = { top: 100, left: 100 }

// ProviderActionMenu uses <Teleport to="body">; content is rendered in document.body, not in wrapper.
const getBodyText = () => document.body.textContent ?? ''
const getBodyButtons = () => Array.from(document.body.querySelectorAll('button'))

describe('ProviderActionMenu — spark shadow 按钮可见性', () => {
  it('删除位于菜单末尾，传递当前提供商并关闭菜单', async () => {
    const provider = makeProvider({ type: 'apikey' })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })
    const deleteButton = getBodyButtons().at(-1)!
    expect(deleteButton.textContent).toContain('common.delete')
    deleteButton.click()
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('delete')?.[0]).toEqual([provider])
    expect(wrapper.emitted('close')).toHaveLength(1)
    wrapper.unmount()
  })

  it('点击高级调度评分入口会携带提供商触发事件', async () => {
    const provider = makeProvider({ platform: 'gemini', type: 'oauth', parent_provider_id: null })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })

    const scoreButton = getBodyButtons().find(button => button.textContent?.includes('admin.providers.advancedSchedulerScore.action'))
    expect(scoreButton).toBeDefined()

    scoreButton!.click()
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('advanced-scheduler-score')?.[0]?.[0]).toMatchObject({ id: provider.id, name: provider.name })
    wrapper.unmount()
  })

  it('普通提供商显示「复制提供商」按钮', () => {
    const provider = makeProvider({ platform: 'anthropic', type: 'apikey', parent_provider_id: null })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })
    expect(getBodyText()).toContain('admin.providers.duplicateProvider')
    wrapper.unmount()
  })

  it('影子提供商隐藏「复制提供商」按钮', () => {
    const provider = makeProvider({ platform: 'openai', type: 'oauth', parent_provider_id: 42 })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })
    expect(getBodyText()).not.toContain('admin.providers.duplicateProvider')
    wrapper.unmount()
  })

  it.each(['oauth', 'setup-token'] as const)('%s 提供商隐藏「复制提供商」按钮，避免共享可轮换令牌', (type) => {
    const provider = makeProvider({ platform: 'openai', type, parent_provider_id: null })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })
    expect(getBodyText()).not.toContain('admin.providers.duplicateProvider')
    wrapper.unmount()
  })

  it('点击「复制提供商」触发 duplicate 事件并携带 provider', async () => {
    const provider = makeProvider({ platform: 'anthropic', type: 'apikey', parent_provider_id: null })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })

    const duplicateBtn = getBodyButtons().find(b => b.textContent?.includes('admin.providers.duplicateProvider'))
    expect(duplicateBtn).toBeDefined()

    duplicateBtn!.click()
    await wrapper.vm.$nextTick()

    const emitted = wrapper.emitted('duplicate')
    expect(emitted).toBeTruthy()
    expect(emitted![0][0]).toMatchObject({ id: provider.id, name: provider.name })
    wrapper.unmount()
  })

  it('OpenAI OAuth 母提供商（无 parent_provider_id）显示「创建 spark 影子」按钮', () => {
    const provider = makeProvider({ platform: 'openai', type: 'oauth', parent_provider_id: null })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })
    expect(getBodyText()).toContain('admin.providers.createSparkShadow')
    wrapper.unmount()
  })

  it('影子提供商（parent_provider_id 非 null）隐藏「创建 spark 影子」按钮', () => {
    const provider = makeProvider({ platform: 'openai', type: 'oauth', parent_provider_id: 42 })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })
    expect(getBodyText()).not.toContain('admin.providers.createSparkShadow')
    wrapper.unmount()
  })

  it('非 OpenAI 提供商隐藏「创建 spark 影子」按钮', () => {
    const provider = makeProvider({ platform: 'antigravity', type: 'oauth', parent_provider_id: null })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })
    expect(getBodyText()).not.toContain('admin.providers.createSparkShadow')
    wrapper.unmount()
  })

  it('影子提供商隐藏凭据/隐私类操作(重授权/刷新token/隐私)— 外审 G4', () => {
    const provider = makeProvider({ platform: 'openai', type: 'oauth', parent_provider_id: 42 })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })
    const body = getBodyText()
    expect(body).not.toContain('admin.providers.reAuthorize')
    expect(body).not.toContain('admin.providers.refreshToken')
    expect(body).not.toContain('admin.providers.setPrivacy')
    wrapper.unmount()
  })

  it('普通 OpenAI OAuth 母提供商仍显示凭据/隐私类操作', () => {
    const provider = makeProvider({ platform: 'openai', type: 'oauth', parent_provider_id: null })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })
    const body = getBodyText()
    expect(body).toContain('admin.providers.reAuthorize')
    expect(body).toContain('admin.providers.setPrivacy')
    wrapper.unmount()
  })

  it('Qoder COSY 提供商显示刷新 token 但不显示重授权入口', () => {
    const provider = makeProvider({
      platform: 'qoder',
      type: 'cosy',
      parent_provider_id: null,
      credentials_status: { has_refresh_token: true },
    })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })
    const body = getBodyText()
    expect(body).toContain('admin.providers.refreshToken')
    expect(body).not.toContain('admin.providers.reAuthorize')
    wrapper.unmount()
  })

  it('无 refresh_token 的 Qoder COSY 提供商隐藏刷新 token 入口', () => {
    const provider = makeProvider({
      platform: 'qoder',
      type: 'cosy',
      parent_provider_id: null,
      credentials_status: { has_pat: true },
    })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })
    expect(getBodyText()).not.toContain('admin.providers.refreshToken')
    wrapper.unmount()
  })

  it('点击按钮触发 create-spark-shadow 事件并携带 provider', async () => {
    const provider = makeProvider({ platform: 'openai', type: 'oauth', parent_provider_id: null })
    const wrapper = mount(ProviderActionMenu, {
      props: { show: true, provider, position },
      attachTo: document.body,
    })

    // Content is teleported to body — find button by text there
    const sparkBtn = getBodyButtons().find(b => b.textContent?.includes('admin.providers.createSparkShadow'))
    expect(sparkBtn).toBeDefined()

    sparkBtn!.click()
    await wrapper.vm.$nextTick()

    const emitted = wrapper.emitted('create-spark-shadow')
    expect(emitted).toBeTruthy()
    expect(emitted![0][0]).toMatchObject({ id: provider.id, platform: 'openai' })

    wrapper.unmount()
  })
})
