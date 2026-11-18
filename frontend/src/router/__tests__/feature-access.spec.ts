import { beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'

type NavigationGuard = (
  to: Record<string, any>,
  from: Record<string, any>,
  next: ReturnType<typeof vi.fn>
) => Promise<void>

const routerHarness = vi.hoisted(() => ({
  guard: null as NavigationGuard | null,
  routes: [] as Array<{ path: string; alias?: string; meta?: Record<string, unknown> }>,
}))

const authStore = vi.hoisted(() => ({
  user: { id: 9 },
  checkAuth: vi.fn(),
  isAuthenticated: true,
  isAdmin: false,
  hasPendingAuthSession: false,
}))

const appStore = vi.hoisted(() => ({
  siteName: 'TokenRouter',
  backendModeEnabled: false,
  publicSettingsLoaded: false,
  cachedPublicSettings: null as null | {
    payment_enabled?: boolean
    risk_control_enabled?: boolean
    team_enabled?: boolean
    usage_ranking_enabled?: boolean
    custom_menu_items?: []
  },
  fetchPublicSettings: vi.fn(),
}))

const readVisibility = vi.hoisted(() => vi.fn())
vi.mock('@/composables/usePelicanVisibility', () => ({ readPelicanVisibility: readVisibility }))

vi.mock('vue-router', () => ({
  createWebHistory: vi.fn(() => ({})),
  createRouter: vi.fn((options: { routes: typeof routerHarness.routes }) => {
    routerHarness.routes = options.routes
    return {
      beforeEach: vi.fn((guard: NavigationGuard) => {
        routerHarness.guard = guard
      }),
      afterEach: vi.fn(),
      onError: vi.fn(),
    }
  }),
}))

vi.mock('@/stores/auth', () => ({
  useAuthStore: () => authStore,
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appStore,
}))

vi.mock('@/stores/adminSettings', () => ({
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
}))

vi.mock('@/stores/adminCompliance', () => ({
  useAdminComplianceStore: () => ({
    initialized: true,
    fetchStatus: vi.fn(),
    requireAcknowledgement: vi.fn(),
  }),
}))

vi.mock('@/composables/useNavigationLoading', () => ({
  useNavigationLoadingState: () => ({
    startNavigation: vi.fn(),
    endNavigation: vi.fn(),
    isLoading: { value: false },
  }),
}))

vi.mock('@/composables/useRoutePrefetch', () => ({
  useRoutePrefetch: () => ({
    triggerPrefetch: vi.fn(),
    cancelPendingPrefetch: vi.fn(),
    resetPrefetchState: vi.fn(),
  }),
}))

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function runGuard(meta: Record<string, unknown>, path: string) {
  if (!routerHarness.guard) {
    throw new Error('router guard was not registered')
  }

  const next = vi.fn()
  const navigation = routerHarness.guard(
    {
      path,
      fullPath: path,
      name: 'FeatureRoute',
      params: {},
      meta: { requiresAuth: true, ...meta },
    },
    {},
    next
  )
  return { navigation, next }
}

describe('feature route guard', () => {
  beforeAll(async () => {
    await import('@/router')
  })

  beforeEach(() => {
    readVisibility.mockReset()
    authStore.isAuthenticated = true
    authStore.isAdmin = false
    appStore.publicSettingsLoaded = false
    appStore.cachedPublicSettings = null
    appStore.fetchPublicSettings.mockReset()
  })

  it('要求登录后查看鹈鹕测智，管理计划另需管理员身份', () => {
    expect(routerHarness.routes.find(route => route.path === '/pelican-tests')?.meta)
      .toMatchObject({ requiresAuth: true, requiresAdmin: false })
    expect(routerHarness.routes.find(route => route.path === '/admin/pelican-tests')?.meta)
      .toMatchObject({ requiresAuth: true, requiresAdmin: true })
  })

  it('用户展示关闭时两个直链都返回工作台，管理端仍可配置展示', async () => {
    const userRoute = routerHarness.routes.find(route => route.path === '/pelican-tests')
    expect(userRoute?.alias).toBe('/pelican-showcase')
    expect(userRoute?.meta?.requiresPelicanShowcase).toBe(true)
    readVisibility.mockResolvedValue({ enabled: false, api_enabled: true })
    for (const path of ['/pelican-tests', '/pelican-showcase']) {
      const blocked = runGuard(userRoute!.meta!, path)
      await blocked.navigation
      expect(blocked.next).toHaveBeenCalledWith('/dashboard')
    }
    authStore.isAdmin = true
    const adminRoute = routerHarness.routes.find(route => route.path === '/admin/pelican-tests')
    const admin = runGuard(adminRoute!.meta!, '/admin/pelican-tests')
    await admin.navigation
    expect(admin.next).toHaveBeenCalledWith()
    readVisibility.mockResolvedValue({ enabled: true, api_enabled: false })
    const open = runGuard(userRoute!.meta!, '/pelican-tests')
    await open.navigation
    expect(open.next).toHaveBeenCalledWith()
  })

  it('waits for the first public-settings request before deciding payment access', async () => {
    const deferred = createDeferred<{ payment_enabled: boolean }>()
    appStore.fetchPublicSettings.mockImplementation(async () => {
      const settings = await deferred.promise
      appStore.cachedPublicSettings = settings
      appStore.publicSettingsLoaded = true
      return settings
    })

    const { navigation, next } = runGuard({ requiresPayment: true }, '/purchase')

    await vi.waitFor(() => expect(appStore.fetchPublicSettings).toHaveBeenCalledTimes(1))
    expect(next).not.toHaveBeenCalled()

    deferred.resolve({ payment_enabled: true })
    await navigation
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it.each([
    ['payment', { requiresPayment: true }, '/purchase'],
    ['risk control', { requiresRiskControl: true }, '/admin/risk-control'],
    ['team', { requiresTeam: true }, '/team'],
    ['usage ranking', { requiresUsageRanking: true }, '/usage-ranking'],
    ['creative', { requiresCreative: true }, '/creative'],
  ])('does not treat a failed %s settings load as explicitly disabled', async (_name, meta, path) => {
    authStore.isAdmin = path.startsWith('/admin/')
    appStore.fetchPublicSettings.mockResolvedValue(null)

    const { navigation, next } = runGuard(meta, path)
    await navigation

    expect(appStore.publicSettingsLoaded).toBe(false)
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith()
  })

  it.each([
    ['payment', { requiresPayment: true }, { payment_enabled: false }, '/dashboard', false],
    [
      'risk control',
      { requiresRiskControl: true },
      { risk_control_enabled: false },
      '/admin/settings',
      true,
    ],
    ['team', { requiresTeam: true }, { team_enabled: false }, '/dashboard', false],
    ['admin team', { requiresTeam: true }, { team_enabled: false }, '/admin/settings', true],
    ['usage ranking', { requiresUsageRanking: true }, { usage_ranking_enabled: false }, '/dashboard', false],
    ['creative', { requiresCreative: true }, { creative_enabled: false }, '/dashboard', false],
    ['admin creative', { requiresCreative: true }, { creative_enabled: false }, '/admin/settings', true],
  ])('redirects when loaded settings explicitly disable %s', async (_name, meta, settings, target, isAdmin) => {
    authStore.isAdmin = isAdmin
    appStore.cachedPublicSettings = settings
    appStore.publicSettingsLoaded = true

    const { navigation, next } = runGuard(meta, '/feature')
    await navigation

    expect(appStore.fetchPublicSettings).not.toHaveBeenCalled()
    expect(next).toHaveBeenCalledOnce()
    expect(next).toHaveBeenCalledWith(target)
  })

  it('请求诊断只注册管理员路由，普通用户直接访问会被拦截', async () => {
    expect(routerHarness.routes.find(route => route.path === '/requests')).toBeUndefined()
    const route = routerHarness.routes.find(route => route.path === '/admin/requests')
    expect(route?.meta?.requiresAdmin).toBe(true)
    const blocked = runGuard(route?.meta ?? {}, '/admin/requests')
    await blocked.navigation
    expect(blocked.next).toHaveBeenCalledWith('/dashboard')
    authStore.isAdmin = true
    const allowed = runGuard(route?.meta ?? {}, '/admin/requests')
    await allowed.navigation
    expect(allowed.next).toHaveBeenCalledWith()
  })
})
