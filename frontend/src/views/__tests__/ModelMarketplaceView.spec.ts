import { defineComponent, nextTick } from 'vue'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { routeLocationKey } from 'vue-router'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import ModelMarketplaceView from '../ModelMarketplaceView.vue'
import { modelIconData } from '@/utils/modelIconData'
import type { MarketplaceGroup, MarketplaceModelPricing } from '@/types'

const getMarketplaceModels = vi.hoisted(() => vi.fn())
const checkAuth = vi.hoisted(() => vi.fn())
const auth = vi.hoisted(() => ({ isAuthenticated: true }))
const fetchPublicSettings = vi.hoisted(() => vi.fn())
const copyToClipboard = vi.hoisted(() => vi.fn())

vi.mock('@/api/marketplace', () => ({
  getMarketplaceModels,
}))

vi.mock('@/composables/useClipboard', () => ({
  useClipboard: () => ({ copied: { value: false }, copyToClipboard }),
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({
    isAuthenticated: auth.isAuthenticated,
    isAdmin: false,
    checkAuth,
  }),
  useAppStore: () => ({
    siteName: 'TokenRouter',
    siteLogo: '',
    docUrl: '',
    cachedPublicSettings: null,
    publicSettingsLoaded: true,
    fetchPublicSettings,
  }),
}))

vi.mock('@/composables/useTheme', () => ({
  initTheme: vi.fn(),
  useTheme: () => ({ isDark: { value: true }, toggleTheme: vi.fn() }),
}))

vi.mock('@/composables/useBalanceDisplay', () => ({
  useBalanceDisplay: () => ({
    balanceUnitName: { value: '点' },
  }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, string> | number) => {
        if (typeof params === 'number') {
          return `${key} ${params}`
        }
        if (key === 'marketplace.rateMultiplierValue') {
          return `marketplace.rateMultiplierValue ${params?.multiplier || ''}`
        }
        if (key === 'marketplace.imageRateMultiplierValue') {
          return `marketplace.imageRateMultiplierValue ${params?.multiplier || ''}`
        }
        if (key === 'marketplace.maxDiscountOff') {
          return `marketplace.maxDiscountOff ${params?.percent || ''}`
        }

        return key
      },
    }),
  }
})

const SelectStub = defineComponent({
  name: 'TokenSelectStub',
  props: {
    modelValue: {
      type: [String, Number, Boolean, null],
      default: null,
    },
    options: {
      type: Array,
      default: () => [],
    },
  },
  emits: ['update:modelValue', 'change'],
  template: `
    <div class="select-stub">
      <button
        v-for="option in options"
        :key="String(option.value)"
        type="button"
        :data-testid="'select-option-' + String(option.value)"
        @click="$emit('update:modelValue', option.value); $emit('change', option.value, option)"
      >
        {{ option.label }}
      </button>
    </div>
  `,
})

const SearchInputStub = defineComponent({
  name: 'SearchInput',
  props: {
    modelValue: {
      type: String,
      default: '',
    },
  },
  emits: ['update:modelValue'],
  template: `
    <input
      data-testid="marketplace-search"
      :value="modelValue"
      @input="$emit('update:modelValue', $event.target.value)"
    />
  `,
})

const tokenPricing: MarketplaceModelPricing = {
  pricing_mode: 'token',
  price_status: 'priced',
  input_price_per_token: 0.000001,
  image_input_price_per_token: 0.000003,
  output_price_per_token: 0.000002,
  fast_image_input_price_per_token: 0.000006,
}

const imagePricing: MarketplaceModelPricing = {
  pricing_mode: 'image',
  price_status: 'priced',
  image_price_1k: 0.5,
}

const unpricedPricing: MarketplaceModelPricing = {
  pricing_mode: 'unknown',
  price_status: 'unpriced',
}

function marketplaceFixture(): MarketplaceGroup[] {
  return [
    marketplaceGroup(1, 'Plus', [
      marketplaceModel('gpt-5.5', 'GPT 5.5', tokenPricing),
      marketplaceModel('legacy-unpriced', 'Legacy Unpriced', unpricedPricing),
    ]),
    marketplaceGroup(2, 'Pro', [
      marketplaceModel('gpt-5.5', 'GPT 5.5', tokenPricing),
    ]),
    marketplaceGroup(3, 'Plus 2', [
      marketplaceModel('gpt-5.5', 'GPT 5.5', tokenPricing),
    ]),
    marketplaceGroup(4, 'Pro 2', [
      marketplaceModel('gpt-5.5', 'GPT 5.5', tokenPricing),
    ]),
  ]
}

function marketplaceGroup(id: number, name: string, models: MarketplaceGroup['models']): MarketplaceGroup {
  return {
    id,
    name,
    description: `${name} group`,
    display_brand: 'OpenAI',
    sort_order: id,
    rate_multiplier: id,

    official_price_ratio: id / 10,
    official_price_rmb_equivalent: id,
    capacity: {
      concurrency_used: id,
      concurrency_max: 10,
      sessions_used: id,
      sessions_max: 20,
      rpm_used: id,
      rpm_max: 60,
    },
    model_count: models.length,
    models,
  }
}

function marketplaceModel(id: string, displayName: string, pricing: MarketplaceModelPricing): MarketplaceGroup['models'][number] {
  return {
    id,
    display_name: displayName,
    pricing,
  }
}

async function mountMarketplace() {
  const wrapper = mount(ModelMarketplaceView, {
    global: {
      provide: { [routeLocationKey as symbol]: { path: '/models' } },
      stubs: {
        AppLayout: { template: '<div><slot name="page-heading-actions" /><slot /></div>' },
        RouterLink: { template: '<a><slot /></a>' },
        Icon: { template: '<span />' },
        LoadingSpinner: { template: '<span />' },
        LocaleSwitcher: { template: '<span />' },
        GroupCapacityBadge: { template: '<span data-testid="group-capacity" />' },
        SearchInput: SearchInputStub,
        Select: SelectStub,
      },
    },
  })
  await flushPromises()
  return wrapper
}

// HelpTooltip 通过 Teleport 挂到 body，隐藏时用 v-show（display:none）控制。
function visibleTooltips() {
  return Array.from(document.body.querySelectorAll<HTMLElement>('[role="tooltip"]'))
    .filter((el) => el.style.display !== 'none')
}

describe('ModelMarketplaceView', () => {
  enableAutoUnmount(afterEach)

  beforeEach(() => {
    localStorage.clear()
    auth.isAuthenticated = true
    getMarketplaceModels.mockReset()
    getMarketplaceModels.mockResolvedValue(marketplaceFixture())
    checkAuth.mockClear()
    fetchPublicSettings.mockClear()
    copyToClipboard.mockClear()
  })

  it('取数期间保留模型卡片占位，不提前显示空状态', async () => {
    let resolveModels!: (value: MarketplaceGroup[]) => void
    getMarketplaceModels.mockImplementationOnce(() => new Promise(resolve => { resolveModels = resolve }))
    const wrapper = await mountMarketplace()
    expect(wrapper.find('[data-testid="marketplace-skeleton"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('marketplace.emptyTitle')

    resolveModels(marketplaceFixture())
    await flushPromises()
    expect(wrapper.find('[data-testid="marketplace-skeleton"]').exists()).toBe(false)
    expect(wrapper.findAll('aside button[aria-pressed]')).toHaveLength(4)
    expect(wrapper.findAll('[data-testid="marketplace-group-section"]')).toHaveLength(1)
  })

  it.each([true, false])('按后台顺序选中首组，切换同名模型时读取分组报价（登录状态 %s）', async (isAuthenticated) => {
    auth.isAuthenticated = isAuthenticated
    const codex = marketplaceGroup(42, 'Codex Pro (负载均衡)', [
      marketplaceModel('shared', 'Shared', tokenPricing),
      marketplaceModel('codex-only', 'Codex Only', tokenPricing),
    ])
    const domestic = marketplaceGroup(17, '国模分组', [
      marketplaceModel('shared', 'Shared', { ...tokenPricing, input_price_per_token: 0.000009 }),
      marketplaceModel('glm-only', 'GLM Only', unpricedPricing),
    ])
    codex.sort_order = 1
    getMarketplaceModels.mockResolvedValue([domestic, codex])
    const wrapper = await mountMarketplace()
    const buttons = wrapper.findAll('aside button[aria-pressed]')
    expect(buttons.map(button => button.attributes('aria-pressed'))).toEqual(['true', 'false'])
    expect(buttons[0].text()).toContain(codex.name)
    const section = () => wrapper.get('[data-testid="marketplace-group-section"]')
    expect(section().get('h2').text()).toBe(codex.name)
    expect(section().findAll('article h3').map(title => title.text())).toEqual(['Shared', 'Codex Only'])
    const firstCard = () => section().get('article')
    expect(firstCard().get('dl').text()).toContain('1.00 点')
    await firstCard().get('[data-testid="model-pricing-toggle"]').trigger('click')
    expect(firstCard().get('[data-testid="pricing-rows"]').text()).toContain('1.00 点')

    await buttons[1].trigger('click')
    expect(buttons.map(button => button.attributes('aria-pressed'))).toEqual(['false', 'true'])
    expect(section().get('h2').text()).toBe(domestic.name)
    expect(section().findAll('article h3').map(title => title.text())).toEqual(['Shared', 'GLM Only'])
    expect(firstCard().get('dl').text()).toContain('9.00 点')
    expect(firstCard().get('[data-testid="model-pricing-toggle"]').attributes('aria-expanded')).toBe('false')
    await firstCard().get('[data-testid="model-pricing-toggle"]').trigger('click')
    expect(firstCard().get('[data-testid="pricing-rows"]').text()).toContain('9.00 点')
    expect(getMarketplaceModels).toHaveBeenCalledTimes(1)
  })

  it('搜索和筛选显示各组的匹配数，手动切组后才显示其他组的结果', async () => {
    const fixture = marketplaceFixture()
    fixture[1].search_terms = ['Domestic Models']
    fixture[1].display_brand = '智谱'
    getMarketplaceModels.mockResolvedValue(fixture)
    const wrapper = await mountMarketplace()
    const section = () => wrapper.get('[data-testid="marketplace-group-section"]')
    const search = wrapper.get('[data-testid="marketplace-search"]')
    await search.setValue('DOMESTIC')

    expect(section().get('h2').text()).toBe('Plus')
    expect(section().get('[role="status"]').text()).toBe('marketplace.modelCount 0')
    expect(section().findAll('article')).toHaveLength(0)
    expect(section().text()).toContain('marketplace.otherGroupsMatch')
    const buttons = wrapper.findAll('aside button[aria-pressed]')
    expect(buttons.map(button => button.attributes('aria-pressed'))).toEqual(['true', 'false'])
    expect(buttons[1].text()).toContain('Pro')
    expect(buttons[1].text()).toContain('1')

    // 窄屏的分组选择框和左栏按钮选择同一个分组。
    await wrapper.get('aside [data-testid="select-option-2"]').trigger('click')
    expect(section().get('h2').text()).toBe('Pro')
    expect(section().get('[role="status"]').text()).toBe('marketplace.modelCount 1')
    expect(section().get('article h3').text()).toBe('GPT 5.5')

    await search.setValue(' GPT-5.5 ')
    await wrapper.get('[data-testid="select-option-智谱"]').trigger('click')
    await wrapper.get('[data-testid="select-option-unpriced"]').trigger('click')
    expect(section().text()).toContain('marketplace.emptyTitle')
    expect(section().text()).not.toContain('marketplace.otherGroupsMatch')
    await wrapper.get('[data-testid="filter-reset"]').trigger('click')
    expect((search.element as HTMLInputElement).value).toBe(' GPT-5.5 ')
    expect(section().findAll('article')).toHaveLength(1)
    expect(section().get('h2').text()).toBe('Pro')
    await search.setValue('no-such-model')
    await section().get('.empty-state button').trigger('click')
    expect((search.element as HTMLInputElement).value).toBe('')
    expect(section().get('h2').text()).toBe('Pro')
  })

  it('刷新后按 ID 保持选中，分组被移除时选中首个可用分组', async () => {
    const fixture = marketplaceFixture()
    const wrapper = await mountMarketplace()
    await wrapper.findAll('aside button[aria-pressed]')[1].trigger('click')
    const reload = async (next: MarketplaceGroup[]) => {
      getMarketplaceModels.mockResolvedValueOnce(next)
      window.dispatchEvent(new Event('locale-changed'))
      await flushPromises()
    }
    const section = () => wrapper.get('[data-testid="marketplace-group-section"]')
    await reload([fixture[3], { ...fixture[1], name: '专业组' }, fixture[0]])
    expect(section().get('h2').text()).toBe('专业组')
    await reload([fixture[3], fixture[0]])
    expect(section().get('h2').text()).toBe('Plus')
    await reload([])
    expect(wrapper.find('article').exists()).toBe(false)
    expect(wrapper.text()).toContain('marketplace.noPublicGroups')
    expect(wrapper.find('.empty-state button').exists()).toBe(false)
  })

  it('迟到的目录响应不能替换新返回的分组和价格', async () => {
    let resolveEarlier!: (value: MarketplaceGroup[]) => void
    getMarketplaceModels.mockImplementationOnce(() => new Promise(resolve => { resolveEarlier = resolve }))
    const wrapper = await mountMarketplace()
    getMarketplaceModels.mockResolvedValueOnce([marketplaceGroup(7, 'New', [marketplaceModel('new', 'New Model', tokenPricing)])])
    window.dispatchEvent(new Event('locale-changed'))
    await flushPromises()
    resolveEarlier(marketplaceFixture())
    await flushPromises()
    expect(wrapper.get('[data-testid="marketplace-group-section"] h2').text()).toBe('New')
    expect(wrapper.get('article h3').text()).toBe('New Model')
  })

  it('没有模型的分组保留分组名和切换入口', async () => {
    getMarketplaceModels.mockResolvedValue([
      marketplaceGroup(1, '空分组', []),
      marketplaceGroup(2, '图片', [marketplaceModel('image', 'Image', imagePricing)]),
    ])
    const wrapper = await mountMarketplace()
    const section = () => wrapper.get('[data-testid="marketplace-group-section"]')
    expect(section().get('h2').text()).toBe('空分组')
    expect(section().text()).toContain('marketplace.emptyGroup')
    expect(section().find('.empty-state button').exists()).toBe(false)
    await wrapper.findAll('aside button[aria-pressed]')[1].trigger('click')
    expect(section().get('article h3').text()).toBe('Image')
  })

  it('展示当前分组的可用率', async () => {
    const fixture = marketplaceFixture()
    fixture[0] = {
      ...fixture[0],
      availability: {
        window_days: 7,
        bucket_minutes: 1440,
        success_count: 6,
        total_count: 7,
        availability_rate: 6 / 7,
        days: [],
      },
    }
    getMarketplaceModels.mockResolvedValue(fixture)

    const wrapper = await mountMarketplace()

    expect(wrapper.findAll('[data-testid="group-capacity"]')).toHaveLength(0)
    expect(wrapper.get('[data-testid="marketplace-group-availability"]').text()).toContain('85.71%')
    await wrapper.findAll('aside button[aria-pressed]')[1].trigger('click')
    expect(wrapper.find('[data-testid="marketplace-group-availability"]').exists()).toBe(false)
  })

  it('当前分组有折扣时显示最高优惠标签和说明', async () => {
    const fixture = marketplaceFixture()
    // 两个分组分别覆盖无折扣和未配置官方价的情况。
    fixture[1] = { ...fixture[1], official_price_ratio: 1.2 }
    fixture[2] = { ...fixture[2], official_price_ratio: undefined }
    getMarketplaceModels.mockResolvedValue(fixture)

    const wrapper = await mountMarketplace()
    const sections = () => wrapper.get('[data-testid="marketplace-group-section"]')
    expect(sections().text()).toContain('marketplace.maxDiscountOff 90')
    await sections().get('[data-testid="group-max-discount-tag"]').trigger('click')
    await nextTick()
    expect(visibleTooltips().some((el) => el.textContent?.includes('marketplace.maxDiscountHint'))).toBe(true)
    await sections().get('[data-testid="group-rate-multiplier-tag"]').trigger('click')
    await nextTick()
    expect(visibleTooltips().some((el) => el.textContent?.includes('marketplace.rateMultiplierHint'))).toBe(true)

    const buttons = wrapper.findAll('aside button[aria-pressed]')
    for (const button of buttons.slice(1, 3)) {
      await button.trigger('click')
      expect(sections().find('[data-testid="group-max-discount-tag"]').exists()).toBe(false)
    }
    await buttons[3].trigger('click')
    expect(sections().text()).toContain('marketplace.maxDiscountOff 60')
  })

  it('模型卡片摘要保留免费默认区间', async () => {
    const fixture = marketplaceFixture()
    fixture[0].models[0].pricing = { pricing_mode: 'token', price_status: 'priced', context_intervals: [
      { min_tokens: 0, max_tokens: 100 },
      { min_tokens: 100, max_tokens: null, input_price_per_token: 0.000002 },
    ] }
    getMarketplaceModels.mockResolvedValue(fixture)
    const wrapper = await mountMarketplace()
    const section = wrapper.findAll('[data-testid="marketplace-group-section"]')[0]
    expect(section.text()).toContain('0.00')
    expect(section.text()).toContain('0-100')
    expect(section.text()).toContain('100+')
  })

  it('模型卡片中的模型 ID 支持一键复制', async () => {
    const wrapper = await mountMarketplace()

    // 卡片 ID 行的复制按钮直接复制模型 ID。
    const cardCopyButtons = wrapper.findAll('[data-testid="model-id-copy"]')
    expect(cardCopyButtons.length).toBeGreaterThan(0)
    await cardCopyButtons[0].trigger('click')
    expect(copyToClipboard).toHaveBeenCalledWith('gpt-5.5')
  })

  it('模型卡片右上角展示输入输出能力标签', async () => {
    const wrapper = await mountMarketplace()

    const sections = wrapper.findAll('[data-testid="marketplace-group-section"]')
    const gptCards = sections[0].findAll('article')
    // fixture 中 gpt-5.5 定价数据带图片输入价，应展示 文字·图片 -> 文字。
    expect(gptCards[0].get('[data-testid="model-capability-tags"]').findAll('[data-modality]').map((tag) => tag.attributes('data-modality')))
      .toEqual(['input-text', 'input-image', 'output-text'])
    // 无定价数据的模型回退为纯文本能力。
    expect(gptCards[1].get('[data-testid="model-capability-tags"]').findAll('[data-modality]').map((tag) => tag.attributes('data-modality')))
      .toEqual(['input-text', 'output-text'])
  })

  it('能力标签优先使用接口下发的模态元数据', async () => {
    const fixture = marketplaceFixture()
    fixture[0] = {
      ...fixture[0],
      models: [
        // 本地规则会把 gpt-image-2 识别为 text+image 输入，接口数据应覆盖为纯文字输入。
        { ...marketplaceModel('gpt-image-2', 'GPT Image 2', imagePricing), input_modalities: ['text'], output_modalities: ['image'] },
      ],
    }
    getMarketplaceModels.mockResolvedValue(fixture)

    const wrapper = await mountMarketplace()
    const sections = wrapper.findAll('[data-testid="marketplace-group-section"]')

    expect(sections[0].get('[data-testid="model-capability-tags"]').findAll('[data-modality]').map((tag) => tag.attributes('data-modality')))
      .toEqual(['input-text', 'output-image'])
  })

  it('Gemini Flash 档位展示接口下发的完整音视频输入能力', async () => {
    const fixture = marketplaceFixture()
    // 档位 ID 保持不变，图标完全采用后端解析的能力元数据。
    fixture[0].models = ['gemini-3.7-flash-tiered', 'gemini-3.8-flash-tiered'].map((id) => ({
      ...marketplaceModel(id, id, imagePricing),
      input_modalities: ['text', 'image', 'audio', 'video'],
      output_modalities: ['text'],
    }))
    getMarketplaceModels.mockResolvedValue(fixture)
    const wrapper = await mountMarketplace()
    const cards = wrapper.findAll('[data-testid="marketplace-group-section"]')[0].findAll('article')
    expect(cards).toHaveLength(2)
    for (const card of cards) {
      expect(card.get('[data-testid="model-capability-tags"]').findAll('[data-modality]').map((tag) => tag.attributes('data-modality')))
        .toEqual(['input-text', 'input-image', 'input-audio', 'input-video', 'output-text'])
    }
  })

  it('模型信息浮层按目录顺序列出可用协议并标出原生协议，没有属性时也显示入口', async () => {
    getMarketplaceModels.mockResolvedValueOnce([
      marketplaceGroup(1, 'Plus', [
        { ...marketplaceModel('gpt-5.5', 'GPT 5.5', tokenPricing), protocols: ['openai_chat_completions', 'anthropic_messages'], native_protocols: ['openai_chat_completions'] },
        marketplaceModel('legacy-unpriced', 'Legacy Unpriced', unpricedPricing),
      ]),
    ])
    const wrapper = await mountMarketplace()
    const triggers = wrapper.findAll('[data-testid="model-attributes-trigger"]')
    expect(triggers).toHaveLength(1)

    await triggers[0].trigger('click')
    await nextTick()
    const tooltip = visibleTooltips().find((el) => el.querySelector('[data-testid="model-protocols"]'))
    expect(tooltip).toBeDefined()
    const chips = Array.from(tooltip!.querySelectorAll<HTMLElement>('[data-protocol]'))
    expect(chips.map((chip) => chip.dataset.protocol)).toEqual(['anthropic_messages', 'openai_chat_completions'])
    expect(chips.map((chip) => chip.title)).toEqual([
      'POST /v1/messages · marketplace.protocolConvertedHint',
      'POST /v1/chat/completions · marketplace.protocolNativeHint',
    ])
    expect(chips.map((chip) => chip.dataset.native)).toEqual(['false', 'true'])
    expect(tooltip!.querySelector('[data-testid="model-protocols-native-legend"]')).not.toBeNull()
    expect(chips[0].textContent).toContain('marketplace.protocolNames.anthropic_messages')
  })

  it('xAI 分组展示品牌图形', async () => {
    const fixture = marketplaceFixture()
    fixture[0] = {
      ...fixture[0],
      name: 'Grok',
      display_brand: 'xAI',
    }
    getMarketplaceModels.mockResolvedValue(fixture)

    const wrapper = await mountMarketplace()

    // 品牌名 xAI 应映射到现有 Grok SVG，不能退回紫色字母 X。
    const grokGroup = wrapper.findAll('[data-testid="marketplace-group-section"]')
      .find((section) => section.get('h2').text() === 'Grok')
    expect(grokGroup?.find('.provider-icon[width="28px"]').exists()).toBe(true)
    expect(grokGroup?.find('.provider-icon-fallback').exists()).toBe(false)
  })

  // 分组名和展示品牌都可以提供智谱名称，两个尺寸的图标共用品牌映射。
  it.each([
    ['GLM', ''],
    ['zai', ''],
    ['Z.AI', ''],
    ['zai Pro', ''],
    ['自定义分组', '智谱'],
    ['自定义分组', 'zai'],
    ['自定义分组', 'z.ai'],
  ])('分组 %s、展示品牌 %s 显示智谱图标', async (name, displayBrand) => {
    getMarketplaceModels.mockResolvedValue([{
      ...marketplaceGroup(1, name, [marketplaceModel('glm-5', 'GLM 5', tokenPricing)]),
      display_brand: displayBrand,
    }])

    const wrapper = await mountMarketplace()
    const group = wrapper.get('[data-testid="marketplace-group-section"]')

    for (const size of ['14px', '28px']) {
      const icon = group.get(`svg.provider-icon[width="${size}"]`)
      expect(icon.findAll('path').map((path) => path.attributes('d'))).toEqual(modelIconData.zhipu.paths)
    }
    expect(group.find('.provider-icon-fallback').exists()).toBe(false)
    expect(group.text()).toContain('智谱')
  })

  it('只展示统一分组倍率', async () => {
    const fixture = marketplaceFixture()
    fixture[0] = {
      ...fixture[0],

      models: [
        marketplaceModel('gpt-image-1', 'GPT Image', imagePricing),
      ],
    }
    getMarketplaceModels.mockResolvedValue(fixture)

    const wrapper = await mountMarketplace()

    expect(wrapper.findAll('[data-testid="marketplace-group-section"]').map((section) => section.text()).join('\n')).not.toContain('marketplace.imageRateMultiplierValue')
  })

  it('模型卡片可展开抽屉式定价面板并切换 fast mode', async () => {
    const wrapper = await mountMarketplace()

    const toggle = wrapper.get('[data-testid="model-pricing-toggle"]')
    expect(toggle.attributes('aria-expanded')).toBe('false')

    await toggle.trigger('click')
    await nextTick()

    // 面板原地展开，展示完整定价（标准计费行），且 fast mode 切换可用。
    const sections = wrapper.findAll('[data-testid="marketplace-group-section"]')
    const firstCard = sections[0].findAll('article')[0]
    expect(firstCard.text()).toContain('marketplace.input')
    expect(firstCard.text()).toContain('marketplace.imageInput')
    expect(firstCard.text()).toContain('marketplace.output')

    const fastSwitch = firstCard.get('[data-testid="pricing-fast-switch"]')
    await fastSwitch.findAll('button')[1].trigger('click')
    await nextTick()

    // fast mode 下展示 fast 加价行，标准行隐藏。
    expect(firstCard.text()).toContain('marketplace.fastImageInput')
    expect(firstCard.text()).not.toContain('marketplace.imageInput')

    await wrapper.get('[data-testid="model-pricing-toggle"]').trigger('click')
    await nextTick()
    expect(wrapper.get('[data-testid="model-pricing-toggle"]').attributes('aria-expanded')).toBe('false')
  })
})
