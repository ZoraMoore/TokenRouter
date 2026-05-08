<template>
  <div v-if="hasProviders" class="space-y-4">
    <div v-if="showDivider" class="flex items-center gap-3">
      <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
      <span class="text-xs text-gray-500 dark:text-dark-400">
        {{ t('auth.oauthOrContinue') }}
      </span>
      <div class="h-px flex-1 bg-gray-200 dark:bg-dark-700"></div>
    </div>

    <div :class="providerGridClass">
      <button
        v-for="provider in visibleProviders"
        :key="provider"
        type="button"
        :disabled="disabled"
        class="btn btn-secondary h-12 w-full justify-center gap-2"
        @click="startLogin(provider)"
      >
        <GitHubMark v-if="provider === 'github'" class="h-5 w-5 text-gray-800 dark:text-gray-100" />
        <GoogleMark v-else class="h-5 w-5" />
        <span class="font-medium">{{ providerLabel(provider) }}</span>
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useI18n } from 'vue-i18n'
import GitHubMark from './GitHubMark.vue'
import GoogleMark from './GoogleMark.vue'

type EmailOAuthProvider = 'github' | 'google'
const EMAIL_OAUTH_PENDING_PROVIDER_KEY = 'email_oauth_pending_provider'
const EMAIL_OAUTH_PENDING_REFERRAL_KEY = 'email_oauth_referral_code'

const props = withDefaults(defineProps<{
  disabled?: boolean
  referralCode?: string
  githubEnabled?: boolean
  googleEnabled?: boolean
  showDivider?: boolean
}>(), {
  showDivider: true
})

const route = useRoute()
const { t } = useI18n()

const visibleProviders = computed<EmailOAuthProvider[]>(() => {
  const providers: EmailOAuthProvider[] = []
  if (props.githubEnabled) providers.push('github')
  if (props.googleEnabled) providers.push('google')
  return providers
})

const hasProviders = computed(() => visibleProviders.value.length > 0)
const hasMultipleProviders = computed(() => visibleProviders.value.length > 1)
const providerGridClass = computed(() => [
  'grid',
  'grid-cols-1',
  'gap-3',
  hasMultipleProviders.value ? 'sm:grid-cols-2' : ''
])

function providerLabel(provider: EmailOAuthProvider): string {
  const name = provider === 'github' ? 'GitHub' : 'Google'
  return hasMultipleProviders.value ? name : t('auth.emailOAuth.signIn', { providerName: name })
}

function firstQueryValue(value: unknown): string {
  if (typeof value === 'string') return value.trim()
  if (Array.isArray(value)) {
    const item = value.find((entry) => typeof entry === 'string' && entry.trim())
    return typeof item === 'string' ? item.trim() : ''
  }
  return ''
}

function resolvedReferralCode(): string {
  return (props.referralCode?.trim() || firstQueryValue(route.query.ref)).trim()
}

function startLogin(provider: EmailOAuthProvider): void {
  const redirectTo = (route.query.redirect as string) || '/dashboard'
  const referralCode = resolvedReferralCode()
  window.sessionStorage.setItem(EMAIL_OAUTH_PENDING_PROVIDER_KEY, provider)
  if (referralCode) {
    window.sessionStorage.setItem(EMAIL_OAUTH_PENDING_REFERRAL_KEY, referralCode)
  } else {
    window.sessionStorage.removeItem(EMAIL_OAUTH_PENDING_REFERRAL_KEY)
  }
  const apiBase = (import.meta.env.VITE_API_BASE_URL as string | undefined) || '/api/v1'
  const normalized = apiBase.replace(/\/$/, '')
  const params = new URLSearchParams({ redirect: redirectTo })
  if (referralCode) {
    params.set('ref', referralCode)
  }
  window.location.href = `${normalized}/auth/oauth/${provider}/start?${params.toString()}`
}
</script>
