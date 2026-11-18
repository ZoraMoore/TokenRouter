<template>
  <div v-if="visible" class="space-y-1">
    <div class="flex flex-wrap items-center gap-1.5">
      <slot name="pre-actions" />

      <!-- 提供商列表展示重置次数，并提供查询入口。 -->
      <button
        type="button"
        class="inline-flex min-w-[54px] items-center justify-center gap-0.5 rounded-compact px-1.5 py-0.5 text-xs font-medium text-primary-600 transition-colors hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-500 dark:hover:bg-primary-500/8"
        :disabled="loading || resetting"
        :title="countButtonTitle"
        @click="handleQuery()"
      >
        <Icon
          name="refresh"
          size="xs"
          :class="{ 'animate-spin': loading }"
          :stroke-width="2"
        />
        <span>{{ t('admin.providers.openaiQuotaReset.count') }}</span>
        <span v-if="data">{{ availableResetCount }}</span>
      </button>

      <template v-if="!isShadow">
        <button type="button" data-testid="quota-reset" class="inline-flex items-center gap-1 rounded-compact px-1.5 py-0.5 text-xs font-medium text-amber-700 hover:bg-amber-50 disabled:opacity-50 dark:text-amber-400 dark:hover:bg-amber-900/20"
          :disabled="!canReset" :title="resetTitle" @click="confirming = true">
          {{ t('common.reset') }}
        </button>
        <button type="button" data-testid="quota-invite" class="inline-flex items-center gap-1 rounded-compact px-1.5 py-0.5 text-xs font-medium text-primary-600 hover:bg-primary-50 disabled:opacity-50 dark:text-primary-500 dark:hover:bg-primary-500/8"
          :disabled="resetting" @click="emit('invite-reset', provider)">
          {{ t('admin.providers.openaiQuotaReset.invite') }}
        </button>
      </template>
      <slot />
    </div>

    <div v-if="primaryResetCreditExpiry" class="space-y-1">
      <div class="flex flex-wrap items-center gap-1">
        <span
          class="inline-flex max-w-full items-center rounded-compact bg-gray-100 px-1.5 py-0.5 text-xs leading-4 text-gray-600 tabular-nums dark:bg-dark-800 dark:text-gray-300"
          :title="t('admin.providers.openaiQuotaReset.expiresAtFull', { time: formatResetCreditExpiry(primaryResetCreditExpiry, 'full') })"
        >
          {{ t('admin.providers.openaiQuotaReset.expiresAt', { time: formatResetCreditExpiry(primaryResetCreditExpiry, 'short') }) }}
        </span>
        <button
          v-if="hiddenResetCreditCount > 0"
          type="button"
          data-testid="reset-credit-expiry-toggle"
          class="inline-flex items-center rounded-compact bg-gray-100 px-1.5 py-0.5 text-xs font-medium leading-4 text-gray-600 transition-colors hover:bg-gray-200 dark:bg-dark-800 dark:text-gray-300 dark:hover:bg-dark-700"
          :aria-expanded="showResetCreditDetails"
          :aria-label="resetCreditDetailsToggleLabel"
          :title="resetCreditDetailsTitle"
          @click="toggleResetCreditDetails"
        >
          +{{ hiddenResetCreditCount }}
        </button>
      </div>

      <div
        v-if="showResetCreditDetails && resetCreditExpirations.length > 1"
        data-testid="reset-credit-expiry-details"
        class="inline-grid max-w-full gap-0.5 rounded-compact border border-gray-200 bg-white px-1.5 py-1 text-xs leading-4 text-gray-600 shadow-sm dark:border-dark-700 dark:bg-dark-900 dark:text-gray-300"
      >
        <span class="sr-only">{{ t('admin.providers.openaiQuotaReset.expirationDetails') }}</span>
        <span
          v-for="(expiresAt, index) in resetCreditExpirations"
          :key="`${expiresAt}-${index}`"
          class="flex min-w-0 items-center gap-1 tabular-nums"
          :title="t('admin.providers.openaiQuotaReset.expiresAtFull', { time: formatResetCreditExpiry(expiresAt, 'full') })"
        >
          <span class="h-1 w-1 shrink-0 rounded-full bg-gray-400 dark:bg-dark-500" />
          <span class="truncate">{{ formatResetCreditExpiry(expiresAt, 'short') }}</span>
        </span>
      </div>
    </div>

    <div
      v-if="error"
      class="max-w-[180px] truncate text-xs text-red-600 dark:text-red-400"
      :title="error"
    >
      {{ truncatedError }}
    </div>
    <div
      v-else-if="warning"
      class="max-w-[220px] text-xs text-amber-600 dark:text-amber-400"
    >
      {{ warning }}
    </div>
    <div v-if="message" class="text-xs text-emerald-700 dark:text-emerald-400" role="status">{{ message }}</div>
    <ConfirmDialog v-if="confirming" :show="confirming" :title="t('admin.providers.openaiQuotaReset.confirmTitle')"
      :message="t('admin.providers.openaiQuotaReset.confirmMessage', { count: availableResetCount })"
      :loading="resetting" @cancel="confirming = false" @confirm="handleReset" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Provider } from '@/types'
import {
  refreshOpenAIQuota,
  resetOpenAIQuota,
  type OpenAIQuotaUsage
} from '@/api/admin/providers'
import Icon from '@/components/icons/Icon.vue'
import ConfirmDialog from '@/components/common/ConfirmDialog.vue'

const props = defineProps<{
  provider: Provider
  refreshToken?: number
}>()
const emit = defineEmits<{
  'invite-reset': [provider: Provider]
  'provider-updated': [provider: Provider]
  'quota-updated': [quota: OpenAIQuotaUsage]
}>()

const { t } = useI18n()

const visible = computed(() => props.provider.platform === 'openai' && props.provider.type === 'oauth')
const loading = ref(false)
const resetting = ref(false)
const confirming = ref(false)
const message = ref('')
const isShadow = computed(() => props.provider.parent_provider_id != null)
let generation = 0
const error = ref<string | null>(null)
const warning = ref<string | null>(null)
const data = ref<OpenAIQuotaUsage | null>(null)
const showResetCreditDetails = ref(false)

// 读取正数缓存快照，其中的到期明细需要仍然有效。
const readCachedResetCredits = (provider: Provider): OpenAIQuotaUsage | null => {
  const cached = provider.extra?.codex_reset_credit_snapshot
  if (!cached || typeof cached !== 'object' || Array.isArray(cached)) return null

  const { available_count: count, credits: rawCredits } = cached as {
    available_count?: unknown
    credits?: unknown
  }
  if (typeof count !== 'number' || !Number.isFinite(count)) return null

  const now = Date.now()
  const credits: { expires_at?: string }[] = []
  if (Array.isArray(rawCredits)) {
    for (const credit of rawCredits) {
      if (!credit || typeof credit !== 'object') continue
      const expiresAt = (credit as { expires_at?: unknown }).expires_at
      if (typeof expiresAt !== 'string' || expiresAt.trim() === '') continue
      const expiryTime = new Date(expiresAt).getTime()
      // 无法解析的到期时间按上游返回值保留并计入可用次数。
      if (!Number.isNaN(expiryTime) && expiryTime <= now) continue
      credits.push({ expires_at: expiresAt })
    }
  }

  const availableCount = Math.min(Math.max(count, 0), credits.length)
  if (count > 0 && availableCount <= 0) return null
  return {
    fetched_at: 0,
    rate_limit_reset_credits: {
      available_count: availableCount,
      credits
    }
  }
}

data.value = readCachedResetCredits(props.provider)

const availableResetCount = computed(() => data.value?.rate_limit_reset_credits?.available_count ?? 0)
const canReset = computed(() => visible.value && !isShadow.value && !loading.value && !resetting.value && availableResetCount.value > 0)
const resetTitle = computed(() => t(`admin.providers.openaiQuotaReset.${!data.value ? 'resetTooltipNeedQuery' : availableResetCount.value > 0 ? 'resetTooltipReady' : 'resetTooltipNoCredits'}`))
const resetCreditExpirations = computed(() =>
  (data.value?.rate_limit_reset_credits?.credits ?? [])
    .map((credit) => credit.expires_at?.trim() ?? '')
    .filter((expiresAt) => expiresAt.length > 0)
    .sort(compareResetCreditExpiry)
)
const primaryResetCreditExpiry = computed(() => resetCreditExpirations.value[0] ?? '')
const hiddenResetCreditCount = computed(() => Math.max(resetCreditExpirations.value.length - 1, 0))

const countButtonTitle = computed(() => {
  if (!data.value) return t('admin.providers.openaiQuotaReset.countTooltipLoad')
  return t('admin.providers.openaiQuotaReset.countTooltipRefresh')
})

const resetCreditDetailsTitle = computed(() =>
  resetCreditExpirations.value
    .map((expiresAt) => formatResetCreditExpiry(expiresAt, 'full'))
    .join('\n')
)

const resetCreditDetailsToggleLabel = computed(() => {
  if (showResetCreditDetails.value) {
    return t('admin.providers.openaiQuotaReset.collapseExpirations')
  }
  return t('admin.providers.openaiQuotaReset.expandExpirations', { count: hiddenResetCreditCount.value })
})

const truncatedError = computed(() => {
  if (!error.value) return ''
  return error.value.length > 80 ? `${error.value.slice(0, 80)}...` : error.value
})

const getResetCreditExpiryTime = (value: string): number => {
  const time = new Date(value).getTime()
  return Number.isNaN(time) ? Number.POSITIVE_INFINITY : time
}

function compareResetCreditExpiry(a: string, b: string): number {
  const diff = getResetCreditExpiryTime(a) - getResetCreditExpiryTime(b)
  if (diff !== 0) return diff
  return a.localeCompare(b)
}

const formatResetCreditExpiry = (value: string, style: 'short' | 'full'): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value

  const options: Intl.DateTimeFormatOptions = {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }
  if (style === 'full') options.year = 'numeric'
  return new Intl.DateTimeFormat(undefined, options).format(date)
}

const extractErrorMessage = (e: unknown): string => {
  const err = e as {
    message?: string
    reason?: string
    response?: { data?: { message?: string; error?: string } }
  }
  return (
    err?.message ||
    err?.reason ||
    err?.response?.data?.message ||
    err?.response?.data?.error ||
    t('common.error')
  )
}

const toggleResetCreditDetails = () => {
  if (hiddenResetCreditCount.value <= 0) return
  showResetCreditDetails.value = !showResetCreditDetails.value
}

const handleQuery = async (refresh = false) => {
  if (!visible.value || resetting.value || (loading.value && !refresh)) return
  const request = ++generation
  loading.value = true
  error.value = null
  warning.value = null
  message.value = ''
  showResetCreditDetails.value = false
  try {
    const result = await refreshOpenAIQuota(props.provider.id)
    if (request !== generation) return
    data.value = result
    emit('quota-updated', result)
    if (!result.cache_persisted) {
      warning.value = t('admin.providers.openaiQuotaReset.refreshCachePersistFailed')
    }
  } catch (e) {
    if (request === generation) error.value = extractErrorMessage(e)
  } finally {
    if (request === generation) loading.value = false
  }
}

const handleReset = async () => {
  if (!confirming.value || !canReset.value) return
  const request = ++generation
  resetting.value = true
  error.value = null
  warning.value = null
  message.value = ''
  try {
    const result = await resetOpenAIQuota(props.provider.id)
    if (request !== generation) return
    data.value = result.quota ?? null
    if (result.provider) emit('provider-updated', result.provider)
    if (result.quota) emit('quota-updated', result.quota)
    if (result.code === 'reset' || result.code === 'success' || ((!result.code || result.code === 'ok') && result.windows_reset > 0)) {
      message.value = t('admin.providers.openaiQuotaReset.resetSuccess', { windows: result.windows_reset })
    } else {
      const outcomes: Record<string, string> = { nothing_to_reset: 'inviteResetNothingToReset', already_redeemed: 'inviteResetAlreadyRedeemed', no_credit: 'inviteResetNoCredit' }
      message.value = t(`admin.providers.${outcomes[result.code] ?? 'inviteResetConsumeUnknown'}`)
    }
    if (result.warning_code) warning.value = t('admin.providers.openaiQuotaReset.resetRefreshWarning')
  } catch (e) {
    if (request === generation) {
      data.value = null
      error.value = extractErrorMessage(e)
    }
  } finally {
    if (request === generation) {
      resetting.value = false
      confirming.value = false
    }
  }
}

watch(
  [() => props.provider.id, visible, isShadow],
  () => {
    generation++
    data.value = readCachedResetCredits(props.provider)
    resetting.value = false
    confirming.value = false
    message.value = ''
    error.value = null
    warning.value = null
    loading.value = false
    showResetCreditDetails.value = false
  },
  { flush: 'sync' }
)

watch(() => props.refreshToken, (token) => { if (token) void handleQuery(true) })
onBeforeUnmount(() => { generation++ })

watch(resetCreditExpirations, () => {
  if (hiddenResetCreditCount.value <= 0) {
    showResetCreditDetails.value = false
  }
})
</script>
