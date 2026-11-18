<template>
  <AuthCardDialog :show="show" @after-leave="$emit('after-leave')" @close="$emit('close')">
        <!-- Header -->
        <div class="mb-6 text-center">
          <h3 class="text-xl font-semibold text-gray-900 dark:text-white">
            {{ t('profile.totp.setupTitle') }}
          </h3>
          <p class="mt-2 text-sm text-gray-500 dark:text-gray-400">
            {{ stepDescription }}
          </p>
        </div>

        <!-- Step 0: Identity Verification -->
        <div v-if="step === 0" v-content-reveal class="space-y-6">
          <!-- Loading verification method -->
          <ContentSkeleton v-if="methodLoading" variant="form" :rows="2" class="py-4" />

          <template v-else>
            <!-- Email verification -->
            <div v-if="verificationMethod === 'email'" class="space-y-4">
              <div>
                <label class="input-label">{{ t('profile.totp.emailCode') }}</label>
                <div class="flex gap-2">
                  <input
                    v-model="verifyForm.emailCode"
                    type="text"
                    maxlength="6"
                    inputmode="numeric"
                    class="input flex-1"
                    :placeholder="t('profile.totp.enterEmailCode')"
                  />
                  <button
                    type="button"
                    class="btn btn-secondary whitespace-nowrap"
                    :disabled="sendingCode || codeCooldown > 0"
                    @click="handleSendCode"
                  >
                    {{ codeCooldown > 0 ? `${codeCooldown}s` : (sendingCode ? t('common.sending') : t('profile.totp.sendCode')) }}
                  </button>
                </div>
              </div>
            </div>

            <!-- Password verification -->
            <div v-else class="space-y-4">
              <div>
                <label class="input-label">{{ t('profile.currentPassword') }}</label>
                <input
                  v-model="verifyForm.password"
                  type="password"
                  autocomplete="current-password"
                  class="input"
                  :placeholder="t('profile.totp.enterPassword')"
                />
              </div>
            </div>

            <div class="flex justify-end gap-3 pt-4">
              <button type="button" class="btn btn-secondary" @click="$emit('close')">
                {{ t('common.cancel') }}
              </button>
              <button
                type="button"
                class="btn btn-primary"
                :disabled="!canProceedFromVerify || setupLoading"
                @click="handleVerifyAndSetup"
              >
                {{ setupLoading ? t('common.loading') : t('common.next') }}
              </button>
            </div>
          </template>
        </div>

        <!-- Step 1: Show QR Code -->
        <div v-if="step === 1" v-content-reveal class="space-y-6">
          <!-- QR Code and Secret -->
          <template v-if="setupData">
            <div class="flex justify-center">
              <div class="rounded-surface border border-gray-200 p-4 bg-white dark:border-dark-600 dark:bg-white">
                <img :src="qrCodeDataUrl" alt="QR Code" class="h-48 w-48" />
              </div>
            </div>

            <div class="text-center">
              <p class="text-sm text-gray-500 dark:text-gray-400 mb-2">
                {{ t('profile.totp.manualEntry') }}
              </p>
              <div class="flex items-center justify-center gap-2">
                <code class="rounded-compact bg-gray-100 px-3 py-2 font-mono text-sm dark:bg-dark-700">
                  {{ setupData.secret }}
                </code>
                <button
                  type="button"
                  class="rounded-compact p-1.5 text-gray-500 hover:bg-gray-100 dark:hover:bg-dark-700"
                  @click="copySecret"
                >
                  <Icon name="clipboard" size="sm" />
                </button>
              </div>
            </div>
          </template>

          <div class="flex justify-end gap-3 pt-4">
            <button type="button" class="btn btn-secondary" @click="$emit('close')">
              {{ t('common.cancel') }}
            </button>
            <button
              type="button"
              class="btn btn-primary"
              :disabled="!setupData"
              @click="step = 2"
            >
              {{ t('common.next') }}
            </button>
          </div>
        </div>

        <!-- Step 2: Verify Code -->
        <div v-if="step === 2" v-content-reveal class="space-y-6">
          <form @submit.prevent="handleVerify">
            <div class="mb-6">
              <label class="input-label text-center block mb-3">
                {{ t('profile.totp.enterCode') }}
              </label>
              <div class="flex justify-center gap-2">
                <input
                  v-for="(_, index) in 6"
                  :key="index"
                  :ref="(el) => setInputRef(el, index)"
                  data-testid="totp-digit-input"
                  type="text"
                  :maxlength="index === 0 ? 6 : 1"
                  inputmode="numeric"
                  :pattern="index === 0 ? '[0-9]{1,6}' : '[0-9]'"
                  :autocomplete="index === 0 ? 'one-time-code' : 'off'"
                  :name="index === 0 ? 'totp_setup_code' : undefined"
                  class="h-12 w-10 rounded-control border border-gray-300 text-center text-lg font-semibold focus:border-primary-500 focus:ring-primary-500 dark:border-dark-600 dark:bg-dark-700"
                  @input="handleCodeInput($event, index)"
                  @change="handleCodeInput($event, index)"
                  @keydown="handleKeydown($event, index)"
                  @paste="handlePaste"
                />
              </div>
            </div>

            <div class="flex justify-end gap-3">
              <button type="button" class="btn btn-secondary" @click="step = 1">
                {{ t('common.back') }}
              </button>
              <button
                type="submit"
                class="btn btn-primary"
                :disabled="verifying || code.join('').length !== 6"
              >
                {{ verifying ? t('common.verifying') : t('profile.totp.verify') }}
              </button>
            </div>
          </form>
        </div>
  </AuthCardDialog>
</template>

<script setup lang="ts">
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import { vContentReveal } from '@/directives/contentReveal'

import Icon from '@/components/icons/Icon.vue'
import AuthCardDialog from '@/components/common/AuthCardDialog.vue'
import { ref, onMounted, onUnmounted, nextTick, watch, computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { useAppStore } from '@/stores/app'
import { totpAPI } from '@/api'
import type { TotpSetupResponse } from '@/types'
import QRCode from 'qrcode'

withDefaults(defineProps<{ show?: boolean }>(), { show: true })

const emit = defineEmits<{
  'after-leave': []
  close: []
  success: []
}>()

const { t } = useI18n()
const appStore = useAppStore()

// 步骤：0 = 身份验证，1 = 显示二维码，2 = 校验 TOTP 验证码。
const step = ref(0)
const methodLoading = ref(true)
const verificationMethod = ref<'email' | 'password'>('password')
const verifyForm = ref({ emailCode: '', password: '' })
const sendingCode = ref(false)
const codeCooldown = ref(0)
const cooldownTimer = ref<ReturnType<typeof setInterval> | null>(null)

const setupLoading = ref(false)
const setupData = ref<TotpSetupResponse | null>(null)
const verifying = ref(false)
const code = ref<string[]>(['', '', '', '', '', ''])
const inputRefs = ref<(HTMLInputElement | null)[]>([])
const qrCodeDataUrl = ref('')

const stepDescription = computed(() => {
  switch (step.value) {
    case 0:
      return verificationMethod.value === 'email'
        ? t('profile.totp.verifyEmailFirst')
        : t('profile.totp.verifyPasswordFirst')
    case 1:
      return t('profile.totp.setupStep1')
    case 2:
      return t('profile.totp.setupStep2')
    default:
      return ''
  }
})

const canProceedFromVerify = computed(() => {
  if (verificationMethod.value === 'email') {
    return verifyForm.value.emailCode.length === 6
  }
  return verifyForm.value.password.length > 0
})

// setupData 变化时重新生成 Base64 二维码。
watch(
  () => setupData.value?.qr_code_url,
  async (url) => {
    if (url) {
      try {
        qrCodeDataUrl.value = await QRCode.toDataURL(url, {
          width: 200,
          margin: 2,
          color: {
            dark: '#000000',
            light: '#ffffff'
          }
        })
      } catch (err) {
        console.error('Failed to generate QR code:', err)
      }
    }
  },
  { immediate: true }
)

const setInputRef = (el: any, index: number) => {
  inputRefs.value[index] = el as HTMLInputElement | null
}

const handleCodeInput = (event: Event, index: number) => {
  const input = event.target as HTMLInputElement
  const digits = input.value.replace(/[^0-9]/g, '').slice(0, 6)

  // 密码管理器可能把完整验证码一次写入当前分格，需要在这里拆回各格。
  if (digits.length > 1) {
    fillCodeDigits(digits.split(''))
    const focusIndex = Math.min(digits.length, 5)
    nextTick(() => inputRefs.value[focusIndex]?.focus())
    return
  }

  const value = digits[0] || ''
  code.value[index] = value
  input.value = value

  if (value && index < 5) {
    nextTick(() => {
      inputRefs.value[index + 1]?.focus()
    })
  }
}

const fillCodeDigits = (digits: string[]) => {
  const nextCode = ['', '', '', '', '', '']
  const availableDigits = digits.slice(0, 6)

  availableDigits.forEach((digit, offset) => {
    nextCode[offset] = digit
  })
  for (let index = availableDigits.length; index < 6; index++) {
    nextCode[index] = ''
  }

  code.value = nextCode
  inputRefs.value.forEach((visibleInput, index) => {
    if (visibleInput) visibleInput.value = nextCode[index]
  })
}

const handleKeydown = (event: KeyboardEvent, index: number) => {
  if (event.key === 'Backspace') {
    const input = event.target as HTMLInputElement
    // 当前格为空且不是首格时，退格回到上一格。
    if (!input.value && index > 0) {
      event.preventDefault()
      inputRefs.value[index - 1]?.focus()
    }
    // 其他情况交给浏览器处理，input 事件会通过 handleCodeInput 同步状态。
  }
}

const handlePaste = (event: ClipboardEvent) => {
  event.preventDefault()
  const pastedData = event.clipboardData?.getData('text') || ''
  const digits = pastedData.replace(/[^0-9]/g, '').slice(0, 6).split('')

  // 同步响应式验证码和可见输入框。
  fillCodeDigits(digits)

  const focusIndex = Math.min(digits.length, 5)
  nextTick(() => {
    inputRefs.value[focusIndex]?.focus()
  })
}

const copySecret = async () => {
  if (setupData.value) {
    try {
      await navigator.clipboard.writeText(setupData.value.secret)
      appStore.showSuccess(t('common.copied'))
    } catch {
      appStore.showError(t('common.copyFailed'))
    }
  }
}

const loadVerificationMethod = async () => {
  methodLoading.value = true
  try {
    const method = await totpAPI.getVerificationMethod()
    verificationMethod.value = method.method
  } catch (err: any) {
    appStore.showError(err.response?.data?.message || t('common.error'))
    emit('close')
  } finally {
    methodLoading.value = false
  }
}

const handleSendCode = async () => {
  sendingCode.value = true
  try {
    await totpAPI.sendVerifyCode()
    appStore.showSuccess(t('profile.totp.codeSent'))
    // Start cooldown
    codeCooldown.value = 60
    if (cooldownTimer.value) {
      clearInterval(cooldownTimer.value)
      cooldownTimer.value = null
    }
    cooldownTimer.value = setInterval(() => {
      codeCooldown.value--
      if (codeCooldown.value <= 0) {
        if (cooldownTimer.value) {
          clearInterval(cooldownTimer.value)
          cooldownTimer.value = null
        }
      }
    }, 1000)
  } catch (err: any) {
    appStore.showError(err.response?.data?.message || t('profile.totp.sendCodeFailed'))
  } finally {
    sendingCode.value = false
  }
}

const handleVerifyAndSetup = async () => {
  setupLoading.value = true

  try {
    const request = verificationMethod.value === 'email'
      ? { email_code: verifyForm.value.emailCode }
      : { password: verifyForm.value.password }

    setupData.value = await totpAPI.initiateSetup(request)
    step.value = 1
  } catch (err: any) {
    appStore.showError(err.response?.data?.message || t('profile.totp.setupFailed'))
  } finally {
    setupLoading.value = false
  }
}

const handleVerify = async () => {
  const totpCode = code.value.join('')
  if (totpCode.length !== 6 || !setupData.value) return

  verifying.value = true

  try {
    await totpAPI.enable({
      totp_code: totpCode,
      setup_token: setupData.value.setup_token
    })
    appStore.showSuccess(t('profile.totp.enableSuccess'))
    emit('success')
  } catch (err: any) {
    appStore.showError(err.response?.data?.message || t('profile.totp.verifyFailed'))
    code.value = ['', '', '', '', '', '']
    inputRefs.value.forEach((input) => {
      if (input) input.value = ''
    })
    nextTick(() => {
      inputRefs.value[0]?.focus()
    })
  } finally {
    verifying.value = false
  }
}

onMounted(() => {
  loadVerificationMethod()
})

onUnmounted(() => {
  if (cooldownTimer.value) {
    clearInterval(cooldownTimer.value)
    cooldownTimer.value = null
  }
})
</script>
