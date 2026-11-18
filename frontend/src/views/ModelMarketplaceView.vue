<template>
  <component
    :is="isAuthenticated ? AppLayout : 'div'"
    :class="isAuthenticated ? '' : 'ba-theme-shell relative min-h-screen overflow-hidden'"
  >
    <template v-if="!isAuthenticated">
      <div class="ba-theme-backdrop pointer-events-none fixed inset-0"></div>

      <header class="site-header relative z-20 border-b border-primary-900/10 px-4 sm:px-6">
        <nav class="mx-auto flex h-[var(--header-h)] max-w-7xl items-center justify-between gap-4">
          <router-link to="/home" class="flex min-w-0 items-center gap-2.5">
            <span class="h-8 w-8 shrink-0 overflow-hidden rounded-control shadow-sm">
              <img :src="siteLogo || '/logo.svg'" alt="Logo" class="h-full w-full object-contain" />
            </span>
            <span class="truncate text-base font-semibold text-gray-950 dark:text-white">{{ siteName }}</span>
          </router-link>

          <div class="flex items-center gap-2 sm:gap-3">
            <div class="hidden items-center gap-5 text-sm font-medium text-gray-600 dark:text-dark-300 md:flex">
              <router-link to="/models" class="transition hover:text-gray-950 dark:hover:text-white">
                {{ t('home.nav.models') }}
              </router-link>
              <a
                v-if="docUrl"
                :href="docUrl"
                target="_blank"
                rel="noopener noreferrer"
                class="transition hover:text-gray-950 dark:hover:text-white"
              >
                {{ t('home.docs') }}
              </a>
            </div>

            <LocaleSwitcher />

            <button
              type="button"
              @click="toggleTheme"
              class="flex rounded-control text-primary-900/90 transition-colors hover:bg-primary-100 hover:text-primary-900 dark:text-dark-100/80 dark:hover:bg-dark-800 dark:hover:text-white btn-icon"
              :title="isDark ? t('home.switchToLight') : t('home.switchToDark')"
            >
              <Icon v-if="isDark" name="sun" size="md" />
              <Icon v-else name="moon" size="md" />
            </button>

            <router-link
              to="/login"
              class="inline-flex items-center rounded-full bg-gray-950 px-4 py-2 text-xs font-semibold text-white transition hover:bg-gray-800 dark:bg-white dark:text-dark-950 dark:hover:bg-dark-200"
            >
              {{ t('home.login') }}
            </router-link>
          </div>
        </nav>
      </header>
    </template>

    <section v-content-reveal="!isAuthenticated && motionRoute?.path"
      :class="isAuthenticated
        ? 'space-y-4'
        : 'relative z-10 px-4 pb-12 pt-6 sm:px-6 lg:px-8'"
    >
      <div :class="isAuthenticated ? 'space-y-4' : 'relative mx-auto max-w-[1400px] space-y-4'">
        <div v-if="!isAuthenticated" class="page-heading mb-4">
          <h1 class="page-title">{{ t('marketplace.title') }}</h1>
          <p class="page-description">{{ t('marketplace.subtitle') }}</p>
        </div>
        <ModelMarketplaceSkeleton v-if="loading && !selectedGroup" />

        <div v-else-if="errorMessage" class="card border-red-200 p-6 dark:border-red-500/30">
          <div class="flex flex-col gap-4 md:flex-row md:items-end md:justify-between">
            <div>
              <h2 class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('common.error') }}</h2>
              <p class="mt-2 text-sm leading-6 text-gray-600 dark:text-dark-300">{{ errorMessage }}</p>
            </div>
            <button class="btn btn-primary" type="button" @click="fetchMarketplace">
              {{ t('common.refresh') }}
            </button>
          </div>
        </div>

        <EmptyState v-else-if="!selectedGroup" class="card" :title="t('marketplace.noPublicGroups')" />

        <div v-else class="grid min-w-0 items-start gap-4 md:grid-cols-[12rem_minmax(0,1fr)] xl:grid-cols-[15rem_minmax(0,1fr)]">
          <aside class="card min-w-0 p-4" aria-labelledby="marketplace-group-list">
            <h2 id="marketplace-group-list" class="mb-3 flex items-center gap-2 text-sm font-semibold text-gray-900 dark:text-dark-50">
              <Icon name="layers" size="sm" class="text-gray-500 dark:text-dark-400" />
              {{ t('marketplace.modelGroups') }}
            </h2>
            <Select
              v-model="selectedGroupId"
              class="md:hidden"
              :options="groupSelectOptions"
              :aria-label="selectedGroup.name"
              searchable
            />
            <div class="hidden min-w-0 gap-1 md:grid">
              <button
                v-for="item in sidebarGroups"
                :key="item.id"
                type="button"
                class="flex min-h-9 w-full items-center gap-2 rounded-control border px-3 py-2 text-left text-sm focus-visible:outline-primary-500"
                :class="item.id === selectedGroup.id
                  ? 'border-primary-200 bg-primary-50 font-semibold text-primary-800 dark:border-primary-500/30 dark:bg-primary-500/10 dark:text-primary-200'
                  : 'border-transparent text-gray-700 hover:bg-gray-50 dark:text-dark-200 dark:hover:bg-dark-800'"
                :aria-pressed="item.id === selectedGroup.id"
                aria-controls="marketplace-models"
                @click="selectedGroupId = item.id"
              >
                <ProviderIcon :brand="groupBrandSource(item)" size="18px" />
                <span class="min-w-0 flex-1 [overflow-wrap:anywhere]">{{ item.name }}</span>
                <span class="shrink-0 rounded-compact bg-black/5 px-2 text-xs leading-5 tabular-nums dark:bg-white/10" aria-hidden="true">{{ item.models.length }}</span>
                <span class="sr-only">{{ t('marketplace.modelCount', item.models.length) }}</span>
              </button>
            </div>
          </aside>
          <section
            id="marketplace-models"
            class="card min-w-0"
            aria-labelledby="marketplace-group-name"
            :aria-busy="loading"
            data-testid="marketplace-group-section"
          >
            <header class="card-header flex flex-col gap-4 p-4 md:px-5 xl:flex-row xl:items-center">
              <div class="min-w-0 flex-1 space-y-3">
                <div class="flex items-start gap-3">
                  <span class="flex h-12 w-12 shrink-0 items-center justify-center rounded-surface border border-gray-200 bg-white dark:border-dark-600 dark:bg-dark-950">
                    <ProviderIcon :brand="groupBrandSource(selectedGroup)" size="28px" />
                  </span>
                  <div class="min-w-0 [overflow-wrap:anywhere]">
                    <h2 id="marketplace-group-name" class="text-lg font-semibold text-gray-950 dark:text-white">{{ selectedGroup.name }}</h2>
                    <p v-if="selectedGroup.description" class="mt-1 text-sm leading-6 text-gray-600 dark:text-dark-300">
                      {{ selectedGroup.description }}
                    </p>
                  </div>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <span :class="brandBadgeClass(selectedGroup)">
                    <ProviderIcon :brand="groupBrandSource(selectedGroup)" size="14px" />
                    {{ groupBrandLabel(selectedGroup) }}
                  </span>
                  <HelpTooltip
                    v-if="formatMaxDiscountOff(selectedGroup.official_price_ratio)"
                    trigger="both"
                    width-class="w-72"
                    :closable="false"
                    :content="t('marketplace.maxDiscountHint')"
                  >
                    <template #trigger>
                      <span
                        data-testid="group-max-discount-tag"
                        class="rounded-full border border-emerald-200 bg-emerald-50 px-3 py-1 text-xs font-semibold text-emerald-700 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-200"
                      >
                        {{ formatMaxDiscountOff(selectedGroup.official_price_ratio) }}
                      </span>
                    </template>
                  </HelpTooltip>
                  <HelpTooltip
                    trigger="both"
                    width-class="w-72"
                    :closable="false"
                    :content="t('marketplace.rateMultiplierHint')"
                  >
                    <template #trigger>
                      <span
                        data-testid="group-rate-multiplier-tag"
                        class="rounded-full border border-gray-200/80 bg-gray-50 px-3 py-1 text-xs font-semibold text-gray-600 dark:border-dark-600 dark:bg-dark-700/60 dark:text-dark-200"
                      >
                        {{ formatRateMultiplierLabel(selectedGroup.rate_multiplier) }}
                      </span>
                    </template>
                  </HelpTooltip>
                </div>
              </div>
              <div
                v-if="selectedGroup.availability"
                class="w-full min-w-0 xl:ml-auto xl:w-2/5 xl:max-w-[560px]"
                data-testid="marketplace-group-availability"
              >
                <GroupAvailabilityBar :availability="selectedGroup.availability" />
              </div>
            </header>

            <div class="flex min-w-0 items-center gap-2 px-4 pt-4 md:px-5">
              <label class="min-w-0 flex-1 sm:max-w-md">
                <span class="sr-only">{{ t('marketplace.searchPlaceholder') }}</span>
                <SearchInput v-model="search" :placeholder="t('marketplace.searchPlaceholder')" :debounce-ms="120" />
              </label>
              <FilterDropdown :active-count="activeFilterCount" :columns="2" keep-mounted @reset="resetPanelFilters">
                <FilterField :label="t('marketplace.filterBrand')">
                  <Select v-model="selectedBrand" :options="brandSelectOptions" :aria-label="t('marketplace.filterBrand')" />
                </FilterField>
                <FilterField :label="t('marketplace.filterPricingMode')">
                  <Select v-model="selectedPricingMode" :options="pricingSelectOptions" :aria-label="t('marketplace.filterPricingMode')" />
                </FilterField>
              </FilterDropdown>
              <p class="ml-auto shrink-0 pl-2 text-xs text-gray-500 dark:text-dark-400" role="status">
                {{ t('marketplace.modelCount', selectedGroup.models.length) }}
              </p>
            </div>

            <!-- 卡片列数取决于分组栏右侧的可用宽度。 -->
            <div v-if="selectedGroup.models.length" class="grid min-w-0 grid-cols-[repeat(auto-fill,minmax(min(100%,17rem),1fr))] items-start gap-4 p-4 md:px-5">
              <article
                v-for="model in selectedGroup.models"
                :key="`${selectedGroup.id}-${model.id}`"
                class="group min-w-0 max-w-full rounded-surface border border-gray-100 bg-gray-50/80 p-4 transition hover:-translate-y-0.5 hover:border-black/20 hover:shadow-sm dark:border-dark-700 dark:bg-dark-950/80 dark:hover:border-primary-500/50"
              >
                <div class="flex min-w-0 flex-wrap items-start justify-between gap-2">
                  <div class="flex min-w-0 flex-1 basis-32 items-center">
                    <h3 class="min-w-0 truncate text-base font-semibold text-gray-950 dark:text-white">{{ model.display_name }}</h3>
                    <!-- 模型属性和可用协议收进标题旁的信息图标，悬停或点击后以浮层展示，不占用卡片高度。 -->
                    <HelpTooltip
                      v-if="model.attributes || marketplaceProtocols(model.protocols).length > 0"
                      trigger="both"
                      width-class="w-72"
                      :closable="false"
                      class="shrink-0"
                    >
                      <template #trigger>
                        <button
                          type="button"
                          data-testid="model-attributes-trigger"
                          class="inline-flex h-5 w-5 items-center justify-center rounded-full text-gray-400 transition-colors hover:text-gray-700 dark:text-dark-500 dark:hover:text-dark-200"
                          :aria-label="t('admin.modelAttributes.details')"
                        >
                          <Icon name="infoCircle" size="sm" class="h-4 w-4" />
                        </button>
                      </template>
                      <ModelAttributesSummary v-if="model.attributes" :attributes="model.attributes" variant="tooltip" />
                      <!-- 有属性时，协议段用和属性浮层相同的分隔线隔开。 -->
                      <ModelProtocolChips
                        :protocols="model.protocols"
                        :native-protocols="model.native_protocols"
                        :class="model.attributes ? 'mt-3 border-t border-white/10 pt-3 dark:border-dark-700' : ''"
                      />
                    </HelpTooltip>
                  </div>
                  <ModelCapabilityTags :model="model" />
                </div>
                <!-- ID 独占整行，右侧能力图标占用标题行。 -->
                <ModelIdLabel :model-id="model.id" class="mt-1" />

                <!-- 价格预览使用无边框列表。 -->
                <div class="mt-4">
                  <template v-if="compactPricingRows(model.pricing).length > 0">
                    <dl class="space-y-2">
                      <div
                        v-for="row in compactPricingRows(model.pricing)"
                        :key="row.key"
                        class="flex items-baseline justify-between gap-3 text-sm"
                      >
                        <dt class="shrink-0 text-gray-500 dark:text-dark-400">{{ row.label }}</dt>
                        <dd class="min-w-0 break-words text-right font-medium tabular-nums [overflow-wrap:anywhere] text-gray-900 dark:text-white">{{ row.value }}</dd>
                      </div>
                    </dl>
                  </template>
                  <p v-else class="text-sm text-gray-400 dark:text-dark-500">
                    {{ t('marketplace.pricingUnavailable') }}
                  </p>

                  <!-- 完整定价使用卡片内抽屉浮窗，组件管理展开、收起、区间和 fast mode 切换。 -->
                  <ModelPricingPanel :model="model" />
                </div>
              </article>
            </div>
            <EmptyState
              v-else
              :title="hasFilters ? t('marketplace.emptyTitle') : t('marketplace.emptyGroup')"
              :description="hasFilters
                ? (sidebarGroups.length > 1 ? t('marketplace.otherGroupsMatch') : t('marketplace.emptyDescription'))
                : undefined"
              :action-text="hasFilters ? t('common.reset') : undefined"
              :action-icon="false"
              @action="resetFilters"
            />
          </section>
        </div>
      </div>
    </section>
  </component>
</template>

<script setup lang="ts">
import { useLocaleRefresh } from '@/composables/useLocaleRefresh'
import { vContentReveal } from '@/directives/contentReveal'
import { useRoute as useMotionRoute } from 'vue-router'
const motionRoute = useMotionRoute()

import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import AppLayout from '@/components/layout/AppLayout.vue'
import Icon from '@/components/icons/Icon.vue'
import ModelMarketplaceSkeleton from '@/components/marketplace/ModelMarketplaceSkeleton.vue'
import LocaleSwitcher from '@/components/common/LocaleSwitcher.vue'
import GroupAvailabilityBar from '@/components/marketplace/GroupAvailabilityBar.vue'
import ModelCapabilityTags from '@/components/marketplace/ModelCapabilityTags.vue'
import ModelAttributesSummary from '@/components/common/ModelAttributesSummary.vue'
import ModelPricingPanel from '@/components/marketplace/ModelPricingPanel.vue'
import ModelProtocolChips from '@/components/marketplace/ModelProtocolChips.vue'
import ProviderIcon from '@/components/common/ProviderIcon.vue'
import HelpTooltip from '@/components/common/HelpTooltip.vue'
import EmptyState from '@/components/common/EmptyState.vue'
import SearchInput from '@/components/common/SearchInput.vue'
import Select from '@/components/common/Select.vue'
import FilterDropdown from '@/components/common/FilterDropdown.vue'
import FilterField from '@/components/common/FilterField.vue'
import ModelIdLabel from '@/components/common/ModelIdLabel.vue'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import { initTheme, useTheme } from '@/composables/useTheme'
import { getMarketplaceModels } from '@/api/marketplace'
import { providerBrandDisplayName, providerBrandFilterKey, resolveProviderBrand } from '@/utils/providerBrand'
import { formatCompactTokenRange } from '@/utils/formatters'
import { formatPriceNumber, pricingKind } from '@/utils/marketplacePricing'
import { marketplaceProtocols } from '@/utils/marketplaceProtocols'
import { sanitizeUrl } from '@/utils/url'
import type { MarketplaceGroup, MarketplaceModelPricing, MarketplacePricingInterval } from '@/types'
import { useAppStore, useAuthStore } from '@/stores'

type PricingFilter = 'all' | 'token' | 'image' | 'unpriced'

interface PricingRow {
  key: string
  label: string
  value: string
}

const { t } = useI18n()
const { balanceUnitName } = useBalanceDisplay()

const appStore = useAppStore()
const authStore = useAuthStore()
const { isDark, toggleTheme } = useTheme()

const groups = ref<MarketplaceGroup[]>([])
const loading = ref(true)
const errorMessage = ref('')
const search = ref('')
const selectedBrand = ref('all')
const selectedPricingMode = ref<PricingFilter>('all')
const selectedGroupId = ref<number | null>(null)
let latestRequest = 0

const isAuthenticated = computed(() => authStore.isAuthenticated)

const siteName = computed(() => appStore.siteName || 'TokenRouter')
const siteLogo = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.site_logo || appStore.siteLogo || '', { allowRelative: true, allowDataUrl: true }))
const docUrl = computed(() => sanitizeUrl(appStore.cachedPublicSettings?.doc_url || appStore.docUrl || ''))

const normalizedSearch = computed(() => search.value.trim().toLowerCase())
const activeFilterCount = computed(() => [selectedBrand.value, selectedPricingMode.value].filter(value => value !== 'all').length)
const hasFilters = computed(() => Boolean(normalizedSearch.value) || activeFilterCount.value > 0)

const sortedGroups = computed(() =>
  [...groups.value].sort((left, right) => {
    const sortDiff = (left.sort_order ?? 0) - (right.sort_order ?? 0)
    if (sortDiff !== 0) {
      return sortDiff
    }
    return left.id - right.id
  })
)

const availableBrands = computed(() => {
  const seen = new Set<string>()
  const brands: string[] = []
  for (const group of sortedGroups.value) {
    const brand = groupBrandLabel(group)
    const key = brandKey(brand)
    if (seen.has(key)) {
      continue
    }
    seen.add(key)
    brands.push(brand)
  }
  return brands
})

const brandSelectOptions = computed(() => [
  { value: 'all', label: t('marketplace.allBrands') },
  ...availableBrands.value.map((brand) => ({
    value: brand,
    label: brand,
  })),
])

const pricingSelectOptions = computed(() => [
  { value: 'all', label: t('marketplace.allTypes') },
  { value: 'token', label: t('marketplace.tokenPricing') },
  { value: 'image', label: t('marketplace.imagePricing') },
  { value: 'unpriced', label: t('marketplace.unpriced') },
])

// 每个分组单独筛选模型，同名模型使用所在分组的报价和协议。
const filteredGroups = computed(() => {
  if (!hasFilters.value) return sortedGroups.value
  const keyword = normalizedSearch.value
  const matchesKeyword = (value: string) => value?.toLowerCase().includes(keyword)
  const brandFilter = selectedBrand.value === 'all' ? null : brandKey(selectedBrand.value)
  return sortedGroups.value.map((group) => {
    const brand = groupBrandLabel(group)
    if (brandFilter && brandKey(brand) !== brandFilter) {
      return { ...group, models: [], model_count: 0 }
    }
    const groupTerms = [group.name, group.description, groupBrandSource(group), brand, ...(group.search_terms || [])]
    const matchesGroup = !keyword || groupTerms.some(matchesKeyword)
    const models = group.models.filter(model => {
      if (selectedPricingMode.value !== 'all' && pricingKind(model.pricing) !== selectedPricingMode.value) return false
      if (matchesGroup) return true
      const modelTerms = [model.id, model.display_name, ...(model.attributes?.search_terms || [])]
      return modelTerms.some(matchesKeyword)
    })
    return { ...group, models, model_count: models.length }
  })
})

const selectedGroup = computed(() => filteredGroups.value.find(group => group.id === selectedGroupId.value))
// 当前分组即使没有匹配模型也列在左栏，由用户选择是否切换到其他命中分组。
const sidebarGroups = computed(() => filteredGroups.value.filter(item =>
  !hasFilters.value || item.id === selectedGroupId.value || item.models.length > 0
))
const groupSelectOptions = computed(() => sidebarGroups.value.map(item => ({
  value: item.id,
  label: `${item.name} (${item.models.length})`,
  search_terms: item.search_terms,
})))

function hasPositiveValue(value?: number | null): value is number {
  return typeof value === 'number' && value > 0
}

function hasContextIntervalPricing(pricing: MarketplaceModelPricing): boolean {
  // 缺价范围由后端排除，已返回的零价区间仍需展示范围标签。
  return (pricing.context_intervals?.length ?? 0) > 0
}

// 筛选面板保留搜索词，空结果下的重置同时清空搜索词。
function resetPanelFilters() {
  selectedBrand.value = 'all'
  selectedPricingMode.value = 'all'
}

function resetFilters() {
  search.value = ''
  resetPanelFilters()
}

function formatMultiplier(multiplier: number): string {
  return `x${multiplier.toFixed(multiplier % 1 === 0 ? 0 : 2)}`
}

// 分组倍率文案由 i18n 按各语言的空格规则拼接。
function formatRateMultiplierLabel(multiplier: number): string {
  return t('marketplace.rateMultiplierValue', { multiplier: formatMultiplier(multiplier) })
}


// 生成分组相对官方价的最高优惠文案，与首页精选卡片使用相同算法。比例缺失、非法或不低于 1（无折扣）时返回 null。
function formatMaxDiscountOff(ratio?: number): string | null {
  if (typeof ratio !== 'number' || !Number.isFinite(ratio) || ratio <= 0 || ratio >= 1) {
    return null
  }
  const percent = new Intl.NumberFormat(undefined, {
    maximumFractionDigits: 1,
  }).format((1 - ratio) * 100)
  return t('marketplace.maxDiscountOff', { percent })
}


function formatPrice(value: number): string {
  return `${formatPriceNumber(value)} ${balanceUnitName.value}`
}

function formatPerMillion(value: number): string {
  return `${formatPrice(value * 1_000_000)} ${t('usage.perMillionTokens')}`
}

function formatCompactPerMillion(value: number): string {
  return formatPriceNumber(value * 1_000_000)
}

function formatPerImage(value: number): string {
  return `${formatPrice(value)} ${t('marketplace.perImage')}`
}

function groupBrandSource(group: Pick<MarketplaceGroup, 'display_brand' | 'name'>): string {
  return group.display_brand?.trim() || group.name
}

function groupBrandLabel(group: Pick<MarketplaceGroup, 'display_brand' | 'name'>): string {
  return providerBrandDisplayName(groupBrandSource(group))
}

function brandKey(label: string): string {
  return providerBrandFilterKey(label)
}

function brandBadgeClass(group: MarketplaceGroup): string {
  const base = 'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-semibold ring-1 ring-inset'
  return `${base} ${resolveProviderBrand(groupBrandSource(group)).badgeClass}`
}

function tokenPricingRowsFromValues(pricing: MarketplaceModelPricing | MarketplacePricingInterval): PricingRow[] {
  const rows: PricingRow[] = []

  if (hasPositiveValue(pricing.input_price_per_token)) {
    rows.push({ key: 'input', label: t('marketplace.input'), value: formatPerMillion(pricing.input_price_per_token) })
  }
  if (hasPositiveValue(pricing.image_input_price_per_token)) {
    rows.push({ key: 'image_input', label: t('marketplace.imageInput'), value: formatPerMillion(pricing.image_input_price_per_token) })
  }
  if (hasPositiveValue(pricing.output_price_per_token)) {
    rows.push({ key: 'output', label: t('marketplace.output'), value: formatPerMillion(pricing.output_price_per_token) })
  }
  if (hasPositiveValue(pricing.cache_write_price_per_token)) {
    rows.push({ key: 'cache_write', label: t('marketplace.cacheWrite'), value: formatPerMillion(pricing.cache_write_price_per_token) })
  }
  if (hasPositiveValue(pricing.cache_write_1h_price_per_token)) {
    rows.push({ key: 'cache_write_1h', label: t('marketplace.cacheWrite1h'), value: formatPerMillion(pricing.cache_write_1h_price_per_token) })
  }
  if (hasPositiveValue(pricing.cache_read_price_per_token)) {
    rows.push({ key: 'cache_read', label: t('marketplace.cacheRead'), value: formatPerMillion(pricing.cache_read_price_per_token) })
  }
  if (hasPositiveValue(pricing.image_output_price_per_token)) {
    rows.push({ key: 'image_output', label: t('marketplace.imageOutput'), value: formatPerMillion(pricing.image_output_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_input_price_per_token)) {
    rows.push({ key: 'fast_input', label: t('marketplace.fastInput'), value: formatPerMillion(pricing.fast_input_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_image_input_price_per_token)) {
    rows.push({ key: 'fast_image_input', label: t('marketplace.fastImageInput'), value: formatPerMillion(pricing.fast_image_input_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_output_price_per_token)) {
    rows.push({ key: 'fast_output', label: t('marketplace.fastOutput'), value: formatPerMillion(pricing.fast_output_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_cache_write_price_per_token)) {
    rows.push({ key: 'fast_cache_write', label: t('marketplace.fastCacheWrite'), value: formatPerMillion(pricing.fast_cache_write_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_cache_write_1h_price_per_token)) {
    rows.push({ key: 'fast_cache_write_1h', label: t('marketplace.fastCacheWrite1h'), value: formatPerMillion(pricing.fast_cache_write_1h_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_cache_read_price_per_token)) {
    rows.push({ key: 'fast_cache_read', label: t('marketplace.fastCacheRead'), value: formatPerMillion(pricing.fast_cache_read_price_per_token) })
  }
  if (hasPositiveValue(pricing.fast_image_output_price_per_token)) {
    rows.push({ key: 'fast_image_output', label: t('marketplace.fastImageOutput'), value: formatPerMillion(pricing.fast_image_output_price_per_token) })
  }

  return rows
}

function compactTokenPricingRows(pricing: MarketplaceModelPricing | MarketplacePricingInterval): PricingRow[] {
  const primaryRows: PricingRow[] = []
  if (hasPositiveValue(pricing.input_price_per_token)) {
    primaryRows.push({ key: 'input', label: t('marketplace.input'), value: formatPerMillion(pricing.input_price_per_token) })
  }
  if (hasPositiveValue(pricing.output_price_per_token)) {
    primaryRows.push({ key: 'output', label: t('marketplace.output'), value: formatPerMillion(pricing.output_price_per_token) })
  }
  if (primaryRows.length > 0) {
    return primaryRows
  }

  const rows = tokenPricingRowsFromValues(pricing)
  if (rows.length === 0) {
    return zeroTokenPricingRows()
  }

  return rows
    .filter((row) => !row.key.startsWith('fast_'))
    .slice(0, 2)
}

function zeroTokenPricingRows(): PricingRow[] {
  return [
    { key: 'input', label: t('marketplace.input'), value: formatPerMillion(0) },
    { key: 'output', label: t('marketplace.output'), value: formatPerMillion(0) },
  ]
}

function compactContextIntervalRows(pricing: MarketplaceModelPricing): PricingRow[] {
  return pricing.context_intervals?.flatMap((interval, index) => {
    const pricedRows = compactIntervalTokenPricingRows(interval)
    const rows = pricedRows.length > 0 ? pricedRows : zeroTokenPricingRows()
    return [{
      key: `compact-${interval.min_tokens}-${interval.max_tokens ?? 'up'}-${index}`,
      label: formatCompactTokenRange(interval.min_tokens, interval.max_tokens),
      value: rows.map((row) => `${row.label} ${row.value}`).join(' / '),
    }]
  }) ?? []
}

function compactIntervalTokenPricingRows(pricing: MarketplacePricingInterval): PricingRow[] {
  const rows: PricingRow[] = []
  if (hasPositiveValue(pricing.input_price_per_token)) {
    rows.push({ key: 'input', label: t('marketplace.input'), value: formatCompactPerMillion(pricing.input_price_per_token) })
  }
  if (hasPositiveValue(pricing.output_price_per_token)) {
    rows.push({ key: 'output', label: t('marketplace.output'), value: formatCompactPerMillion(pricing.output_price_per_token) })
  }
  if (rows.length > 0) {
    return rows
  }

  return tokenPricingRowsFromValues(pricing)
    .filter((row) => !row.key.startsWith('fast_'))
    .slice(0, 2)
}

function compactPricingRows(pricing: MarketplaceModelPricing): PricingRow[] {
  const kind = pricingKind(pricing)
  if (kind === 'token' && hasContextIntervalPricing(pricing)) {
    return compactContextIntervalRows(pricing)
  }
  if (kind === 'token') {
    return compactTokenPricingRows(pricing)
  }
  if (kind === 'image') {
    return imagePricingRows(pricing)
  }
  return []
}

function imagePricingRows(pricing: MarketplaceModelPricing): PricingRow[] {
  const values = [
    { key: '1k', label: '1K', price: pricing.image_price_1k },
    { key: '2k', label: '2K', price: pricing.image_price_2k },
    { key: '4k', label: '4K', price: pricing.image_price_4k },
  ]

  return values.flatMap((item) => {
    if (typeof item.price !== 'number' || !Number.isFinite(item.price) || item.price < 0) {
      return []
    }

    return [{
      key: item.key,
      label: item.label,
      value: formatPerImage(item.price),
    }]
  })
}

async function fetchMarketplace() {
  // 切换语言时可能同时有多个目录请求，页面以最近发出的请求为准。
  const request = ++latestRequest
  loading.value = true
  errorMessage.value = ''

  try {
    const next = await getMarketplaceModels()
    if (request !== latestRequest) return
    groups.value = next
    if (!next.some(item => item.id === selectedGroupId.value)) {
      selectedGroupId.value = sortedGroups.value[0]?.id ?? null
    }
  } catch (error) {
    if (request !== latestRequest) return
    console.error('Failed to load marketplace models:', error)
    errorMessage.value =
      typeof error === 'object' && error !== null && 'message' in error
        ? String(error.message)
        : t('common.unknownError')
  } finally {
    if (request === latestRequest) loading.value = false
  }
}

onMounted(async () => {
  initTheme()
  authStore.checkAuth()
  if (!appStore.publicSettingsLoaded) {
    await appStore.fetchPublicSettings()
  }
  await fetchMarketplace()
})

useLocaleRefresh(fetchMarketplace)
</script>
