<template>
  <BaseDialog
    :show="show"
    :title="t('admin.providers.inviteResetTitle')"
    width="wide"
    @close="handleClose"
  >
    <div v-if="provider" class="space-y-5">
      <div class="flex flex-col gap-3 rounded-surface border border-gray-200 bg-gray-50 p-4 dark:border-dark-600 dark:bg-dark-800 sm:flex-row sm:items-center sm:justify-between">
        <div class="flex items-center gap-3">
          <div class="flex h-11 w-11 shrink-0 items-center justify-center rounded-control bg-emerald-500 text-white">
            <Icon name="gift" size="md" :stroke-width="2" />
          </div>
          <div class="min-w-0">
            <div class="truncate font-semibold text-gray-900 dark:text-white">{{ provider.name }}</div>
            <div class="mt-0.5 text-sm text-gray-500 dark:text-gray-400">
              {{ t('admin.providers.inviteResetSubtitle') }}
            </div>
          </div>
        </div>
        <button type="button" class="btn btn-secondary shrink-0" :disabled="busy" @click="loadStatus()">
          <Icon name="refresh" size="sm" :class="loading && 'animate-spin'" />
          {{ t('admin.providers.inviteResetRefresh') }}
        </button>
      </div>

      <div v-if="loading" class="grid gap-4 md:grid-cols-2" role="status" :aria-label="t('common.loading')" aria-busy="true" data-loading-skeleton>
        <ContentSkeleton variant="detail" :rows="4" class="rounded-control border border-gray-200 p-4 dark:border-dark-600" />
        <ContentSkeleton variant="list" :rows="3" class="rounded-control border border-gray-200 p-4 dark:border-dark-600" />
      </div>

      <template v-else>
        <div class="grid grid-cols-1 gap-4 md:grid-cols-[minmax(0,1fr)_minmax(0,1.15fr)]">
          <section class="space-y-4 rounded-control border border-gray-200 p-4 dark:border-dark-600">
            <div class="flex items-center justify-between gap-3">
              <div>
                <div class="text-sm font-medium text-gray-500 dark:text-gray-400">
                  {{ t('admin.providers.inviteResetAvailable') }}
                </div>
                <div class="mt-2 flex items-end gap-2">
                  <span class="text-4xl font-bold text-gray-900 dark:text-white">{{ availableCount }}</span>
                  <span class="pb-1 text-sm text-gray-500 dark:text-gray-400">{{ t('admin.providers.inviteResetAvailableUnit') }}</span>
                </div>
              </div>
              <div class="flex h-12 w-12 items-center justify-center rounded-control bg-cyan-100 text-cyan-700 dark:bg-cyan-500/15 dark:text-cyan-300">
                <Icon name="refresh" size="md" :stroke-width="2" />
              </div>
            </div>

            <div class="grid grid-cols-1 gap-2 rounded-control bg-gray-50 p-3 text-sm dark:bg-dark-800">
              <div class="flex items-center justify-between gap-3">
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.providers.inviteResetSlots') }}</span>
                <span data-testid="invite-capacity">{{ status?.available_invites ?? '—' }}</span>
              </div>
              <div class="flex items-center justify-between gap-3">
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.providers.inviteResetRewardType') }}</span>
                <span class="text-right font-medium text-gray-900 dark:text-white">{{ rewardTypeLabel }}</span>
              </div>
              <div class="flex items-center justify-between gap-3">
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.providers.inviteResetRewardAmount') }}</span>
                <span class="text-right font-medium text-gray-900 dark:text-white">{{ rewardAmountLabel }}</span>
              </div>
              <div class="flex items-center justify-between gap-3">
                <span class="text-gray-500 dark:text-gray-400">{{ t('admin.providers.inviteResetShouldShow') }}</span>
                <span :class="['rounded-full px-2 py-0.5 text-xs font-medium', shouldShowClass]">{{ shouldShowLabel }}</span>
              </div>

            </div>

            <div v-if="availableCredits.length > 0" class="space-y-3">
              <label class="input-label">{{ t('admin.providers.inviteResetSelectedCredit') }}</label>
              <Select
                v-model="selectedCreditId"
                :options="creditOptions"
                :placeholder="t('admin.providers.inviteResetSelectCredit')"
                :searchable="false"
              />
              <div v-if="selectedCredit" class="rounded-control bg-gray-50 p-3 text-sm text-gray-600 dark:bg-dark-800 dark:text-gray-300">
                <div class="font-medium text-gray-900 dark:text-white">{{ creditTitle(selectedCredit) }}</div>
                <div class="mt-1">{{ creditDescription(selectedCredit) }}</div>
                <div v-if="selectedCredit.expires_at" class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                  {{ t('admin.providers.inviteResetCreditExpiresAtFull', { time: formatCreditExpiry(selectedCredit.expires_at, 'full') }) }}
                </div>
              </div>
              <div
                v-if="creditExpirations.length > 0"
                class="space-y-2 text-xs text-gray-600 dark:text-gray-300"
              >
                <div class="font-medium text-gray-700 dark:text-gray-200">
                  {{ t('admin.providers.inviteResetCreditExpirations') }}
                </div>
                <div class="flex flex-wrap gap-2">
                  <span
                    v-for="(expiresAt, index) in creditExpirations"
                    :key="`${expiresAt}-${index}`"
                    class="inline-flex max-w-full items-center rounded-compact bg-gray-100 px-2 py-1 tabular-nums dark:bg-dark-700"
                    :title="t('admin.providers.inviteResetCreditExpiresAtFull', { time: formatCreditExpiry(expiresAt, 'full') })"
                  >
                    {{ formatCreditExpiry(expiresAt, 'short') }}
                  </span>
                </div>
              </div>
            </div>

            <div v-else class="rounded-control border border-dashed border-gray-300 p-4 text-sm text-gray-500 dark:border-dark-600 dark:text-gray-400">
              {{ t(availableCount > 0 ? 'admin.providers.inviteResetCreditDetailsUnavailable' : 'admin.providers.inviteResetNoCredits') }}
            </div>

            <button
              type="button"
              class="btn btn-primary w-full"
              :disabled="!canConsume"
              @click="handleConsume"
            >
              <Icon name="refresh" size="sm" :class="consuming && 'animate-spin'" />
              {{ consuming ? t('admin.providers.inviteResetUsing') : t('admin.providers.inviteResetUseReset') }}
            </button>

            <button
              type="button"
              class="flex w-full items-center justify-between rounded-control border border-gray-200 px-3 py-2 text-left text-sm font-medium text-gray-700 hover:bg-gray-50 dark:border-dark-600 dark:text-gray-200 dark:hover:bg-dark-700"
              @click="showRules = !showRules"
            >
              <span>{{ t('admin.providers.inviteResetRules') }}</span>
              <Icon
                name="chevronDown"
                size="sm"
                :class="['transition-transform', showRules && 'rotate-180']"
                :animate-on-hover="false"
              />
            </button>
            <div v-if="showRules" class="rounded-control bg-gray-50 p-3 text-sm text-gray-600 dark:bg-dark-800 dark:text-gray-300">
              <ul v-if="rules.length > 0" class="list-disc space-y-1 pl-5">
                <li v-for="rule in rules" :key="rule">{{ rule }}</li>
              </ul>
              <p v-else>{{ t('admin.providers.inviteResetRulesEmpty') }}</p>
            </div>
          </section>

          <section class="space-y-4 rounded-control border border-gray-200 p-4 dark:border-dark-600">
            <div>
              <label class="input-label" for="codex-invite-reset-emails">
                {{ t('admin.providers.inviteResetInviteEmails') }}
              </label>
              <textarea
                id="codex-invite-reset-emails"
                v-model="emailInput"
                :disabled="busy"
                rows="8"
                class="input mt-2 min-h-[180px] resize-y font-mono text-sm leading-6"
                :placeholder="t('admin.providers.inviteResetPlaceholder')"
              ></textarea>
              <p class="mt-2 text-xs text-gray-500 dark:text-gray-400">
                {{ t('admin.providers.inviteResetEmailHint', { max: maxEmails }) }}
              </p>
            </div>

            <label v-if="status?.requires_consent !== false" class="flex items-start gap-2 rounded-control border border-gray-200 p-3 text-sm text-gray-700 dark:border-dark-600 dark:text-gray-300">
              <input
                v-model="consentConfirmed"
                :disabled="busy"
                type="checkbox"
                class="mt-0.5 h-4 w-4 rounded-compact border-gray-300 text-primary-600 focus:ring-primary-500"
              />
              <span>{{ t('admin.providers.inviteResetConsent') }}</span>
            </label>

            <div v-if="inviteUnavailableMessage" class="rounded-control bg-amber-50 p-3 text-sm text-amber-700 dark:bg-amber-500/10 dark:text-amber-300">
              {{ inviteUnavailableMessage }}
            </div>

            <div v-if="refreshFailed" class="text-sm text-amber-700 dark:text-amber-300">{{ t('admin.providers.inviteResetStatusRefreshFailed') }}</div>
            <div v-if="message" :class="['rounded-control p-3 text-sm', messageClass]">
              {{ message }}
            </div>

            <button
              type="button"
              class="btn btn-secondary w-full justify-center"
              :disabled="!canSendInvite"
              @click="handleSendInvite"
            >
              <Icon name="mail" size="sm" :class="sendingInvite && 'animate-pulse'" />
              {{ sendingInvite ? t('admin.providers.inviteResetSending') : t('admin.providers.inviteResetSendInvite') }}
            </button>
          </section>
        </div>
      </template>
    </div>

    <template #footer>
      <div class="flex justify-end">
        <button type="button" class="btn btn-secondary" @click="handleClose">
          {{ t('common.close') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { adminAPI } from '@/api/admin'
import type { Provider } from '@/types'
import type { CodexInviteResetCredit, CodexInviteResetStatus } from '@/api/admin/providers'
import BaseDialog from '@/components/common/BaseDialog.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import Icon from '@/components/icons/Icon.vue'

const maxEmails = 5
const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

const props = defineProps<{
  show: boolean
  provider: Provider | null
}>()

const emit = defineEmits<{
  close: []
  updated: [providerId: number]
}>()

const { t } = useI18n()
const appStore = useAppStore()

const loading = ref(false)
const refreshFailed = ref(false)
let generation = 0
const supported = computed(() => props.provider?.platform === 'openai' && props.provider.type === 'oauth' && props.provider.parent_provider_id == null)
const sendingInvite = ref(false)
const consuming = ref(false)
const busy = computed(() => loading.value || consuming.value || sendingInvite.value)
const status = ref<CodexInviteResetStatus | null>(null)
const selectedCreditId = ref<string | null>(null)
const emailInput = ref('')
const consentConfirmed = ref(false)
const message = ref('')
const messageType = ref<'success' | 'error' | ''>('')
const showRules = ref(false)

const availableCredits = computed(() => {
  return (status.value?.credits ?? [])
    .filter((credit) => {
      const state = credit.status?.toLowerCase()
      return !state || state === 'available'
    })
    .sort((a, b) => {
      // 优先使用最早到期的重置机会。
      const expiryOrder = compareCreditExpiry(a.expires_at ?? '', b.expires_at ?? '')
      return expiryOrder !== 0 ? expiryOrder : a.id.localeCompare(b.id)
    })
})

const availableCount = computed(() => status.value?.available_count ?? availableCredits.value.length)

const rules = computed(() => status.value?.eligibility_rules ?? [])

// 奖励类型和可用重置次数分别来自活动和用量接口。
const rewardTypeLabel = computed(() => {
  if (status.value?.grant_type === 'none') {
    return t('admin.providers.inviteResetGrantTypeNone')
  }
  if (status.value?.grant_type === 'rate_limit_reset') {
    return t('admin.providers.inviteResetGrantTypeRateLimitReset')
  }
  if (status.value?.grant_type === 'workspace_credits') {
    return t('admin.providers.inviteResetGrantTypeWorkspaceCredits')
  }
  return t('admin.providers.inviteResetGrantTypeUnknown')
})

const rewardAmountLabel = computed(() => {
  const amount = status.value?.grant_amount
  return typeof amount === 'number' ? String(amount) : t('admin.providers.inviteResetRewardAmountUnknown')
})

const shouldShowLabel = computed(() => {
  if (status.value?.should_show === true) return t('admin.providers.inviteResetShouldShowTrue')
  if (status.value?.should_show === false) return t('admin.providers.inviteResetShouldShowFalse')
  return t('admin.providers.inviteResetShouldShowUnknown')
})

const shouldShowClass = computed(() => {
  if (status.value?.should_show === true) {
    return 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300'
  }
  if (status.value?.should_show === false) {
    return 'bg-amber-100 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300'
  }
  return 'bg-gray-200 text-gray-600 dark:bg-dark-600 dark:text-gray-300'
})

const creditOptions = computed<SelectOption[]>(() => {
  return availableCredits.value.map((credit, index) => ({
    value: credit.id,
    label: credit.expires_at
      ? `${creditTitle(credit)} #${index + 1} · ${formatCreditExpiry(credit.expires_at, 'short')}`
      : `${creditTitle(credit)} #${index + 1}`
  }))
})

const selectedCredit = computed(() => {
  return availableCredits.value.find((credit) => credit.id === selectedCreditId.value) ?? null
})

const creditExpirations = computed(() => {
  return availableCredits.value
    .map((credit) => credit.expires_at?.trim() ?? '')
    .filter((expiresAt) => expiresAt.length > 0)
    .sort(compareCreditExpiry)
})

const canConsume = computed(() => {
  if (!supported.value || busy.value || availableCount.value <= 0) return false
  // usage 有次数但明细不可用时，允许不带 credit_id 使用上游自动选择模式。
  return availableCredits.value.length === 0 || Boolean(selectedCreditId.value)
})

const inviteUnavailableMessage = computed(() => {
  if (!status.value || status.value.invite_available !== false) return ''
  return status.value.invite_unavailable_message || t('admin.providers.inviteResetInviteUnavailable')
})

const canSendInvite = computed(() => {
  const capacity = status.value?.available_invites
  if (!supported.value || busy.value || !status.value?.invite_available || typeof capacity !== 'number' || capacity <= 0) return false
  if (status.value.requires_consent && !consentConfirmed.value) return false
  try { return parseEmails().length <= capacity } catch { return false }
})

const messageClass = computed(() => {
  if (messageType.value === 'success') {
    return 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/10 dark:text-emerald-300'
  }
  if (messageType.value === 'error') {
    return 'bg-red-50 text-red-700 dark:bg-red-500/10 dark:text-red-300'
  }
  return 'bg-gray-50 text-gray-600 dark:bg-dark-700 dark:text-gray-300'
})

const creditTitle = (credit: CodexInviteResetCredit) => {
  return credit.title || t('admin.providers.inviteResetCreditFallbackTitle')
}

const creditDescription = (credit: CodexInviteResetCredit) => {
  return credit.description || t('admin.providers.inviteResetCreditFallbackDescription')
}

const getCreditExpiryTime = (value: string): number => {
  const time = new Date(value).getTime()
  return Number.isNaN(time) ? Number.POSITIVE_INFINITY : time
}

const compareCreditExpiry = (a: string, b: string): number => {
  const left = getCreditExpiryTime(a)
  const right = getCreditExpiryTime(b)
  if (left !== right) return left - right
  return a.localeCompare(b)
}

const formatCreditExpiry = (value: string, style: 'short' | 'full'): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value

  const options: Intl.DateTimeFormatOptions = {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  }
  if (style === 'full') {
    options.year = 'numeric'
  }
  return new Intl.DateTimeFormat(undefined, options).format(date)
}

// 邀请接口支持逗号、分号、空白和换行分隔，这里先在前端做同样的校验。
const parseEmails = () => {
  const emails = emailInput.value
    .split(/[,\s;]+/)
    .map((item) => item.trim())
    .filter(Boolean)
  const unique = [...new Map(emails.map((email) => [email.toLowerCase(), email])).values()]
  if (unique.length === 0) {
    throw new Error(t('admin.providers.inviteResetEmailsRequired'))
  }
  if (unique.length > maxEmails) {
    throw new Error(t('admin.providers.inviteResetEmailLimit', { max: maxEmails }))
  }
  const invalid = unique.find((email) => email.length > 254 || !emailPattern.test(email))
  if (invalid) {
    throw new Error(t('admin.providers.inviteResetInvalidEmail', { email: invalid }))
  }
  return unique
}

const errorMessage = (error: unknown, fallback: string) => {
  if (error && typeof error === 'object' && 'message' in error && typeof error.message === 'string' && error.message) return error.message
  return t(fallback)
}

const setMessage = (type: 'success' | 'error', text: string) => {
  messageType.value = type
  message.value = text
}

const selectDefaultCredit = () => {
  if (!availableCredits.value.length) {
    selectedCreditId.value = null
    return
  }
  if (!availableCredits.value.some((credit) => credit.id === selectedCreditId.value)) {
    selectedCreditId.value = availableCredits.value[0].id
  }
}

const loadStatus = async (clearMessage = true) => {
  if (!props.provider || !props.show || !supported.value || loading.value) return
  const current = generation
  loading.value = true
  refreshFailed.value = false
  if (clearMessage) {
    message.value = ''
    messageType.value = ''
  }
  try {
    const fresh = await adminAPI.providers.getCodexInviteResetStatus(props.provider.id)
    if (current !== generation) return
    status.value = fresh
    selectDefaultCredit()
  } catch (error) {
    if (current !== generation) return
    status.value = null
    if (clearMessage) setMessage('error', errorMessage(error, 'admin.providers.inviteResetLoadFailed'))
    else refreshFailed.value = true
  } finally {
    if (current === generation) loading.value = false
  }
}

const handleSendInvite = async () => {
  if (!props.provider || !canSendInvite.value || !status.value) return
  const current = generation
  const id = props.provider.id
  sendingInvite.value = true
  try {
    const recipients = parseEmails()
    const result = await adminAPI.providers.sendCodexInviteResetInvite(id, recipients, status.value.referral_key, consentConfirmed.value)
    if (current !== generation) return
    const failed = result.failed_emails ?? []
    if (result.sent_count !== recipients.length - failed.length || failed.some((email) => !recipients.includes(email))) {
      throw new Error(t('admin.providers.inviteResetSendUnknown'))
    }
    emailInput.value = failed.join('\n')
    consentConfirmed.value = false
    if (failed.length > 0) {
      setMessage('error', t('admin.providers.inviteResetInvitePartialFailed', { emails: failed.join(', ') }))
    } else {
      setMessage('success', t('admin.providers.inviteResetInviteSuccess'))
      appStore.showSuccess(t('admin.providers.inviteResetInviteSuccess'))
    }
    await loadStatus(false)
  } catch (error) {
    if (current === generation) {
      // 请求可能已经送达，重新查询资格后才允许再次提交。
      if (status.value) status.value = { ...status.value, available_invites: null }
      consentConfirmed.value = false
      setMessage('error', errorMessage(error, 'admin.providers.inviteResetSendUnknown'))
    }
  } finally {
    if (current === generation) sendingInvite.value = false
  }
}

const consumeSuccessMessage = (code?: string) => {
  if (code === 'reset') return t('admin.providers.inviteResetConsumeSuccess')
  if (code === 'nothing_to_reset') return t('admin.providers.inviteResetNothingToReset')
  if (code === 'already_redeemed') return t('admin.providers.inviteResetAlreadyRedeemed')
  if (code === 'no_credit') return t('admin.providers.inviteResetNoCredit')
  return t('admin.providers.inviteResetConsumeUnknown')
}

const handleConsume = async () => {
  if (!props.provider || !canConsume.value) return
  const current = generation
  const id = props.provider.id
  consuming.value = true
  try {
    const result = await adminAPI.providers.consumeCodexInviteReset(id, selectedCreditId.value ?? undefined)
    emit('updated', id)
    if (current !== generation) return
    const code = result.code === 'success' || ((!result.code || result.code === 'ok') && result.windows_reset > 0) ? 'reset' : result.code
    const text = consumeSuccessMessage(code)
    setMessage(code === 'reset' ? 'success' : 'error', text)
    if (code === 'reset') appStore.showSuccess(text)
    await loadStatus(false)
  } catch (error) {
    if (current === generation) {
      status.value = null
      setMessage('error', errorMessage(error, 'admin.providers.inviteResetConsumeUnknown'))
    }
  } finally {
    if (current === generation) consuming.value = false
  }
}

const resetLocalState = () => {
  status.value = null
  selectedCreditId.value = null
  emailInput.value = ''
  consentConfirmed.value = false
  message.value = ''
  messageType.value = ''
  showRules.value = false
}

const handleClose = () => {
  if (!sendingInvite.value && !consuming.value) emit('close')
}

watch(
  [() => props.show, () => props.provider?.id, supported],
  ([visible]) => {
    generation++
    loading.value = false
    sendingInvite.value = false
    consuming.value = false
    refreshFailed.value = false
    resetLocalState()
    if (visible && supported.value) void loadStatus()
  },
  { immediate: true, flush: 'sync' }
)

// 收件人改变后，管理员需要为新的邮箱重新确认同意。
watch(emailInput, () => { consentConfirmed.value = false }, { flush: 'sync' })
watch(availableCredits, selectDefaultCredit)
</script>
