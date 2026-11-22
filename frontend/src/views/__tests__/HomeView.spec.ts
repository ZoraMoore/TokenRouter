import { enableAutoUnmount, flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { routeLocationKey } from 'vue-router'
import HomeView from '../HomeView.vue'

const { getMarketplaceModels, getMarketplaceStats, appStore } = vi.hoisted(() => ({
  getMarketplaceModels: vi.fn().mockResolvedValue([{ id: 1, name: 'OpenAI', models: [{ id: 'gpt-test' }] }]),
  getMarketplaceStats: vi.fn().mockResolvedValue({ today_tokens: 1200, total_tokens: 23000, total_users: 42, show_supported_models: true }),
  appStore: {
    publicSettingsLoaded: true,
    cachedPublicSettings: {} as { show_public_home_stats?: boolean; home_content?: string },
  },
}))

vi.mock('@/api/marketplace', () => ({ getMarketplaceModels, getMarketplaceStats }))

vi.mock('@/stores', () => ({
  useAppStore: () => appStore,
  useAuthStore: () => ({ isAuthenticated: false, isAdmin: false, checkAuth: vi.fn() }),
}))

vi.mock(import('vue-i18n'), async (importOriginal) => ({
  ...await importOriginal(),
  useI18n: () => ({ t: (key: string) => key, locale: { value: 'en' } }),
}))

// 首页规格只验证卡片和数据请求，其他组件使用占位渲染。
async function mountHome() {
  const wrapper = shallowMount(HomeView, {
    global: {
      provide: { [routeLocationKey as symbol]: { path: '/' } },
      stubs: { RouterLink: true },
    },
  })
  await flushPromises()
  return wrapper
}

enableAutoUnmount(afterEach)
afterEach(() => vi.clearAllMocks())

describe('公开首页统计', () => {
  it.each([undefined, false])('开关为 %s 时隐藏统计并跳过统计请求', async (show_public_home_stats) => {
    appStore.cachedPublicSettings = { show_public_home_stats }
    const wrapper = await mountHome()

    expect(wrapper.text()).not.toContain('home.stats.todayTokens')
    expect(getMarketplaceStats).not.toHaveBeenCalled()
    expect(getMarketplaceModels).toHaveBeenCalledOnce()
  })

  it('开启后显示四项统计和对应数值', async () => {
    appStore.cachedPublicSettings = { show_public_home_stats: true }
    const wrapper = await mountHome()

    expect(getMarketplaceStats).toHaveBeenCalledOnce()
    expect(wrapper.findAll('dt').map(label => label.text())).toEqual([
      'home.stats.todayTokens', 'home.stats.totalTokens', 'home.stats.totalUsers', 'home.stats.supportedModels',
    ])
    expect(wrapper.findAll('dd').map(value => value.text())).toEqual(['1.2K', '23K', '42', '1'])
  })

  it('只展示接口返回的统计项，保留数值为零的卡片', async () => {
    appStore.cachedPublicSettings = { show_public_home_stats: true }
    getMarketplaceStats.mockResolvedValueOnce({ today_tokens: 0, total_users: 42 })
    const wrapper = await mountHome()

    expect(wrapper.findAll('dt').map(label => label.text())).toEqual([
      'home.stats.todayTokens', 'home.stats.totalUsers',
    ])
    expect(wrapper.findAll('dd').map(value => value.text())).toEqual(['0', '42'])
    expect(wrapper.text()).not.toContain('marketplace.modelsStat')
    expect(wrapper.text()).not.toContain('home.providers.modelCount')
  })

  it('全部取消勾选时隐藏统计区', async () => {
    appStore.cachedPublicSettings = { show_public_home_stats: true }
    getMarketplaceStats.mockResolvedValueOnce({})
    const wrapper = await mountHome()

    expect(wrapper.find('dl').exists()).toBe(false)
  })

  it('自定义首页跳过统计请求', async () => {
    appStore.cachedPublicSettings = { show_public_home_stats: true, home_content: '<p>Custom home</p>' }
    const wrapper = await mountHome()

    expect(wrapper.text()).toContain('Custom home')
    expect(wrapper.text()).not.toContain('home.stats.todayTokens')
    expect(getMarketplaceStats).not.toHaveBeenCalled()
  })

  it('首页缓存仍为开启但接口已关闭时隐藏统计', async () => {
    appStore.cachedPublicSettings = { show_public_home_stats: true }
    getMarketplaceStats.mockRejectedValueOnce({ reason: 'PUBLIC_HOME_STATS_DISABLED' })
    const wrapper = await mountHome()

    expect(getMarketplaceStats).toHaveBeenCalledOnce()
    expect(wrapper.text()).not.toContain('home.stats.todayTokens')
    expect(wrapper.text()).not.toContain('home.stats.unavailable')
  })
})
