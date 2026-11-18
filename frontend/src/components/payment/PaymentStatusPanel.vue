<template>
  <!-- 支付信息以单列居中展示，并限制宽度。 -->
  <div class="mx-auto w-full max-w-md">
    <!-- 终态：展示结果，由用户确认后返回 -->

    <!-- 支付成功 -->
    <div v-if="outcome === 'success'" class="card p-6 text-center sm:p-8">
      <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-green-500/10">
        <Icon name="check" size="lg" class="text-green-600 dark:text-green-400" :animate-on-hover="false" />
      </div>
      <p class="mt-4 text-lg font-semibold text-gray-900 dark:text-white">
        {{ props.orderType === 'subscription' ? t('payment.result.subscriptionSuccess') : t('payment.result.success') }}
      </p>
      <dl
        v-if="paidOrder"
        class="mt-6 divide-y divide-gray-100 border-y border-gray-100 text-left text-sm dark:divide-dark-700 dark:border-dark-700"
      >
        <div class="flex justify-between gap-4 py-2.5">
          <dt class="text-gray-500 dark:text-dark-400">{{ t('payment.orders.orderId') }}</dt>
          <dd class="font-medium text-gray-900 dark:text-white">#{{ paidOrder.id }}</dd>
        </div>
        <div v-if="paidOrder.out_trade_no" class="flex justify-between gap-4 py-2.5">
          <dt class="shrink-0 text-gray-500 dark:text-dark-400">{{ t('payment.orders.orderNo') }}</dt>
          <dd class="min-w-0 break-all text-right font-mono text-xs leading-5 text-gray-900 dark:text-white">{{ paidOrder.out_trade_no }}</dd>
        </div>
        <div class="flex justify-between gap-4 py-2.5">
          <dt class="text-gray-500 dark:text-dark-400">{{ t('payment.orders.amount') }}</dt>
          <dd class="font-medium tabular-nums text-gray-900 dark:text-white">{{ formatOrderAmount(paidOrder.amount, paidOrder.order_type) }}</dd>
        </div>
        <div class="flex justify-between gap-4 py-2.5">
          <dt class="text-gray-500 dark:text-dark-400">{{ t('payment.orders.payAmount') }}</dt>
          <dd class="font-medium tabular-nums text-gray-900 dark:text-white">{{ formatGatewayAmount(paidOrder.pay_amount) }}</dd>
        </div>
      </dl>
      <button class="btn btn-primary mt-6 w-full" @click="handleDone">{{ t('common.confirm') }}</button>
    </div>

    <!-- 已取消 -->
    <div v-else-if="outcome === 'cancelled'" class="card p-6 text-center sm:p-8">
      <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-gray-500/10">
        <Icon name="x" size="lg" class="text-gray-500 dark:text-dark-400" />
      </div>
      <p class="mt-4 text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.qr.cancelled') }}</p>
      <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('payment.qr.cancelledDesc') }}</p>
      <button class="btn btn-primary mt-6 w-full" @click="handleDone">{{ t('common.confirm') }}</button>
    </div>

    <!-- 已过期或失败 -->
    <div v-else-if="outcome === 'expired'" class="card p-6 text-center sm:p-8">
      <div class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-orange-500/10">
        <Icon name="clock" size="lg" class="text-orange-600 dark:text-orange-400" />
      </div>
      <p class="mt-4 text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.qr.expired') }}</p>
      <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('payment.qr.expiredDesc') }}</p>
      <button class="btn btn-primary mt-6 w-full" @click="handleDone">{{ t('common.confirm') }}</button>
    </div>

    <!-- 支付渠道已受理，等待异步终态。 -->
    <div v-else-if="isProcessing" class="card p-6 text-center sm:p-8">
      <Icon name="loader" size="xl" class="mx-auto animate-spin text-primary-500" :animate-on-hover="false" />
      <p class="mt-4 text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.result.processing') }}</p>
      <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('payment.result.processingHint') }}</p>
    </div>

    <!-- 等待中：二维码或新窗口支付 -->

    <!-- 移动端支付宝唤起；超时前保持二维码兜底隐藏。 -->
    <template v-else-if="isMobileAlipayDeepLink">
      <div v-if="!deepLinkFallbackVisible" class="card overflow-hidden">
        <div class="p-6 text-center sm:p-8">
          <Icon
            v-if="deepLinkState === 'launching'"
            name="loader"
            size="xl"
            class="mx-auto animate-spin text-[#00AEEF]"
            :animate-on-hover="false"
          />
          <div
            v-else
            class="mx-auto flex h-12 w-12 items-center justify-center rounded-full bg-[#00AEEF]/10"
          >
            <Icon name="checkCircle" size="lg" class="text-[#00AEEF]" :animate-on-hover="false" />
          </div>
          <p class="mt-4 text-lg font-semibold text-gray-900 dark:text-white">
            {{ deepLinkState === 'backgrounded' ? t('payment.qr.alipayContinueInApp') : t('payment.qr.alipayOpening') }}
          </p>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('payment.qr.alipayWaitingHint') }}</p>
          <button
            v-if="deepLinkState === 'backgrounded'"
            data-test="reopen-alipay"
            class="btn btn-alipay mt-5 inline-flex items-center gap-2 text-sm"
            @click="reopenAlipay"
          >
            <Icon name="externalLink" size="sm" />
            {{ t('payment.qr.reopenAlipay') }}
          </button>
        </div>
        <div class="flex items-center justify-between gap-4 border-t border-gray-100 px-6 py-3 text-sm dark:border-dark-700">
          <span class="flex items-center gap-2 text-gray-500 dark:text-dark-400">
            <span class="h-2 w-2 animate-pulse rounded-full bg-primary-500" aria-hidden="true"></span>
            {{ t('payment.qr.waitingPayment') }}
          </span>
          <span class="text-gray-500 dark:text-dark-400">
            {{ t('payment.qr.expiresIn') }}
            <span class="ml-1 font-semibold tabular-nums text-gray-900 dark:text-white">{{ countdownDisplay }}</span>
          </span>
        </div>
      </div>
      <div v-else data-test="alipay-qr-fallback" class="card p-6">
        <div class="text-center">
          <p class="text-lg font-semibold text-gray-900 dark:text-white">{{ t('payment.qr.alipayFallbackTitle') }}</p>
          <p class="mt-1 text-sm text-gray-500 dark:text-dark-400">{{ t('payment.qr.alipayFallbackHint') }}</p>
        </div>
        <dl class="mt-5 divide-y divide-gray-100 border-y border-gray-100 text-sm dark:divide-dark-700 dark:border-dark-700">
          <div class="flex items-start justify-between gap-4 py-2.5">
            <dt class="text-gray-500 dark:text-dark-400">{{ t('payment.orders.payAmount') }}</dt>
            <dd class="font-semibold tabular-nums text-gray-900 dark:text-white">{{ displayPaymentAmount }}</dd>
          </div>
          <div class="flex items-start justify-between gap-4 py-2.5">
            <dt class="shrink-0 text-gray-500 dark:text-dark-400">{{ t('payment.orders.orderNo') }}</dt>
            <dd class="min-w-0 break-all text-right font-mono text-xs leading-5 text-gray-900 dark:text-white">
              {{ displayOrderNumber }}
            </dd>
          </div>
          <div class="flex items-start justify-between gap-4 py-2.5">
            <dt class="text-gray-500 dark:text-dark-400">{{ t('payment.qr.expiresIn') }}</dt>
            <dd class="font-semibold tabular-nums text-gray-900 dark:text-white">{{ countdownDisplay }}</dd>
          </div>
        </dl>
        <div class="mt-5 flex justify-center">
          <div :class="['relative rounded-control border bg-white p-3', qrBorderClass]">
            <canvas ref="qrCanvas" class="mx-auto"></canvas>
            <div class="pointer-events-none absolute inset-0 flex items-center justify-center">
              <span :class="['rounded-full p-2 shadow ring-2 ring-white', qrLogoBgClass]">
                <img :src="qrLogoIcon" alt="" class="h-5 w-5 brightness-0 invert" />
              </span>
            </div>
          </div>
        </div>
        <p class="mt-4 text-center text-sm leading-6 text-gray-600 dark:text-dark-300">
          {{ t('payment.qr.alipaySaveAndScanHint') }}
        </p>
        <div class="mt-5 grid gap-2 sm:grid-cols-2">
          <button
            data-test="reopen-alipay"
            class="btn btn-alipay inline-flex items-center justify-center gap-2"
            @click="reopenAlipay"
          >
            <Icon name="externalLink" size="sm" />
            {{ t('payment.qr.reopenAlipay') }}
          </button>
          <button
            data-test="save-alipay-qr"
            class="btn btn-secondary inline-flex items-center justify-center gap-2"
            @click="saveQRCode"
          >
            <Icon name="download" size="sm" />
            {{ t('payment.qr.saveQRCode') }}
          </button>
        </div>
        <button class="btn btn-secondary mt-2 w-full" @click="handleDone">
          {{ t('payment.result.backToRecharge') }}
        </button>
      </div>
    </template>

    <!-- 扫码支付 -->
    <div v-else-if="showQRCode" class="card overflow-hidden">
      <div class="p-6 text-center">
        <p class="text-sm font-medium text-gray-500 dark:text-dark-400">{{ scanTitle }}</p>
        <p class="mt-1 text-2xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ displayPaymentAmount }}</p>
        <!-- 二维码始终放在白底上，深色模式下也能被正常识别。 -->
        <div :class="['relative mx-auto mt-5 inline-block rounded-control border bg-white p-3', qrBorderClass]">
          <canvas ref="qrCanvas" class="mx-auto"></canvas>
          <div class="pointer-events-none absolute inset-0 flex items-center justify-center">
            <span :class="['rounded-full p-2 shadow ring-2 ring-white', qrLogoBgClass]">
              <img :src="qrLogoIcon" alt="" class="h-5 w-5 brightness-0 invert" />
            </span>
          </div>
        </div>
        <p v-if="scanHint" class="mt-4 text-sm text-gray-500 dark:text-dark-400">{{ scanHint }}</p>
        <button v-if="payUrl" class="btn btn-secondary mt-4 text-sm" @click="reopenPopup">
          {{ t('payment.qr.openPayWindow') }}
        </button>
      </div>
      <div class="flex items-center justify-between gap-4 border-t border-gray-100 px-6 py-3 text-sm dark:border-dark-700">
        <span class="flex items-center gap-2 text-gray-500 dark:text-dark-400">
          <span class="h-2 w-2 animate-pulse rounded-full bg-primary-500" aria-hidden="true"></span>
          {{ t('payment.qr.waitingPayment') }}
        </span>
        <span class="text-gray-500 dark:text-dark-400">
          {{ t('payment.qr.expiresIn') }}
          <span class="ml-1 font-semibold tabular-nums text-gray-900 dark:text-white">{{ countdownDisplay }}</span>
        </span>
      </div>
      <div class="border-t border-gray-100 p-4 dark:border-dark-700">
        <button class="btn btn-secondary w-full" :disabled="cancelling" @click="handleCancel">
          {{ cancelling ? t('common.processing') : t('payment.qr.cancelOrder') }}
        </button>
      </div>
    </div>

    <!-- 新窗口或跳转支付 -->
    <div v-else class="card overflow-hidden">
      <div class="p-6 text-center sm:p-8">
        <Icon name="loader" size="xl" class="mx-auto animate-spin text-primary-500" :animate-on-hover="false" />
        <p class="mt-4 text-2xl font-semibold tabular-nums text-gray-900 dark:text-white">{{ displayPaymentAmount }}</p>
        <p class="mt-2 text-sm text-gray-500 dark:text-dark-400">{{ t('payment.qr.payInNewWindowHint') }}</p>
        <button v-if="payUrl" class="btn btn-secondary mt-5 text-sm" @click="reopenPopup">
          {{ t('payment.qr.openPayWindow') }}
        </button>
      </div>
      <div class="flex items-center justify-between gap-4 border-t border-gray-100 px-6 py-3 text-sm dark:border-dark-700">
        <span class="flex items-center gap-2 text-gray-500 dark:text-dark-400">
          <span class="h-2 w-2 animate-pulse rounded-full bg-primary-500" aria-hidden="true"></span>
          {{ t('payment.qr.waitingPayment') }}
        </span>
        <span class="font-semibold tabular-nums text-gray-900 dark:text-white">{{ countdownDisplay }}</span>
      </div>
      <div class="border-t border-gray-100 p-4 dark:border-dark-700">
        <button class="btn btn-secondary w-full" :disabled="cancelling" @click="handleCancel">
          {{ cancelling ? t('common.processing') : t('payment.qr.cancelOrder') }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch, onMounted, onUnmounted, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { usePaymentStore } from '@/stores/payment'
import { useAppStore } from '@/stores'
import { paymentAPI } from '@/api/payment'
import { extractI18nErrorMessage } from '@/utils/apiError'
import { getPaymentPopupFeatures, isBuiltInAlipayMethod, isBuiltInWxpayMethod } from '@/components/payment/providerConfig'
import { useBalanceDisplay } from '@/composables/useBalanceDisplay'
import { formatPaymentAmount, normalizePaymentCurrency } from '@/components/payment/currency'
import type { PaymentOrder } from '@/types/payment'
import Icon from '@/components/icons/Icon.vue'
import QRCode from 'qrcode'
import alipayIcon from '@/assets/icons/alipay.svg'
import wxpayIcon from '@/assets/icons/wxpay.svg'
import paymentIcon from '@/assets/icons/payment.svg'
import {
  createAlipayDeepLinkLauncher,
  type AlipayDeepLinkLauncher,
  type AlipayDeepLinkState,
} from './alipayDeepLink'

const props = defineProps<{
  orderId: number
  amount?: number
  payAmount?: number
  qrCode: string
  expiresAt: string
  paymentType: string
  outTradeNo?: string
  payUrl?: string
  orderType?: string
  currency?: string
  mobileAlipayDeepLink?: boolean
}>()

type PaymentOutcome = 'success' | 'cancelled' | 'expired'

const emit = defineEmits<{ done: []; success: []; settled: [outcome: PaymentOutcome] }>()

const i18n = useI18n()
const { t } = i18n
const paymentStore = usePaymentStore()
const appStore = useAppStore()
const { formatBalanceAmount } = useBalanceDisplay()

const qrCanvas = ref<HTMLCanvasElement | null>(null)
const qrUrl = ref('')
const remainingSeconds = ref(0)
const cancelling = ref(false)
const isProcessing = ref(false)
const paidOrder = ref<PaymentOrder | null>(null)
const deepLinkState = ref<AlipayDeepLinkState>('idle')
const deepLinkFallbackVisible = ref(false)
const paymentCurrency = computed(() => normalizePaymentCurrency(paidOrder.value?.currency || props.currency))
const localeCode = computed(() => {
  const raw = i18n.locale as unknown
  if (typeof raw === 'string') return raw
  if (raw && typeof raw === 'object' && 'value' in raw) {
    return String((raw as { value?: string }).value || '')
  }
  return undefined
})

// Terminal outcome: null = still active, 'success' | 'cancelled' | 'expired'
const outcome = ref<PaymentOutcome | null>(null)

let pollTimer: ReturnType<typeof setInterval> | null = null
let countdownTimer: ReturnType<typeof setInterval> | null = null
let pollIntervalMs = 0
let verifyAttempts = 0
let lastVerifyAt = 0
let alipayLauncher: AlipayDeepLinkLauncher | null = null

const PENDING_POLL_INTERVAL_MS = 3000
const PROCESSING_POLL_INTERVAL_MS = 15000
const VERIFY_RETRY_INTERVAL_MS = 15000
const VERIFY_RETRY_MAX_ATTEMPTS = 6

const isAlipay = computed(() => isBuiltInAlipayMethod(props.paymentType))
const isWxpay = computed(() => isBuiltInWxpayMethod(props.paymentType))
const isMobileAlipayDeepLink = computed(() => props.mobileAlipayDeepLink === true && isAlipay.value && !!qrUrl.value)
const showQRCode = computed(() => !!qrUrl.value && (!isMobileAlipayDeepLink.value || deepLinkFallbackVisible.value))

const qrBorderClass = computed(() => {
  if (isAlipay.value) return 'border-[#00AEEF]/40'
  if (isWxpay.value) return 'border-[#2BB741]/40'
  return 'border-gray-200 dark:border-dark-600'
})

const qrLogoBgClass = computed(() => {
  if (isAlipay.value) return 'bg-[#00AEEF]'
  if (isWxpay.value) return 'bg-[#2BB741]'
  return 'bg-gray-400'
})

const qrLogoIcon = computed(() => {
  if (isAlipay.value) return alipayIcon
  if (isWxpay.value) return wxpayIcon
  return paymentIcon
})

const scanTitle = computed(() => {
  if (isAlipay.value) return t('payment.qr.scanAlipay')
  if (isWxpay.value) return t('payment.qr.scanWxpay')
  return t('payment.qr.scanToPay')
})

const scanHint = computed(() => {
  if (isAlipay.value) return t('payment.qr.scanAlipayHint')
  if (isWxpay.value) return t('payment.qr.scanWxpayHint')
  return ''
})

const countdownDisplay = computed(() => {
  const m = Math.floor(remainingSeconds.value / 60)
  const s = remainingSeconds.value % 60
  return m.toString().padStart(2, '0') + ':' + s.toString().padStart(2, '0')
})

const displayPaymentAmount = computed(() => formatGatewayAmount(props.payAmount || props.amount || 0))
const displayOrderNumber = computed(() => props.outTradeNo || `#${props.orderId}`)

function formatOrderAmount(amount: number, orderType: string): string {
  return orderType === 'balance' ? formatBalanceAmount(amount, { fractionDigits: 2 }) : formatGatewayAmount(amount)
}

function formatGatewayAmount(value: number, currency?: string | null): string {
  return formatPaymentAmount(value, currency || paymentCurrency.value, localeCode.value)
}

function isSuccessStatus(status: string | null | undefined): boolean {
  return status === 'COMPLETED' || status === 'PAID' || status === 'RECHARGING'
}

function upstreamVerificationOutTradeNo(): string {
  if (props.paymentType !== 'stripe' && !isWxpay.value && !isMobileAlipayDeepLink.value) return ''
  return props.outTradeNo || ''
}

function reopenPopup() {
  if (props.payUrl) {
    const win = window.open(props.payUrl, 'paymentPopup', getPaymentPopupFeatures())
    if (!win || win.closed) {
      window.location.href = props.payUrl
    }
  }
}

function setOutcome(next: PaymentOutcome) {
  if (outcome.value === next) return
  outcome.value = next
  emit('settled', next)
}

async function renderQR() {
  await nextTick()
  if (!showQRCode.value || !qrCanvas.value || !qrUrl.value) return
  await QRCode.toCanvas(qrCanvas.value, qrUrl.value, {
    width: 220, margin: 2,
    errorCorrectionLevel: 'M',
  })
}

function updateDeepLinkState(state: AlipayDeepLinkState) {
  deepLinkState.value = state
  if (state === 'fallback') {
    deepLinkFallbackVisible.value = true
    renderQR()
  } else if (state === 'backgrounded') {
    deepLinkFallbackVisible.value = false
  }
}

function reopenAlipay() {
  alipayLauncher?.launch()
}

function saveQRCode() {
  const canvas = qrCanvas.value
  if (!canvas) return
  const link = document.createElement('a')
  link.href = canvas.toDataURL('image/png')
  link.download = `alipay-${props.outTradeNo || props.orderId}.png`
  document.body.appendChild(link)
  link.click()
  link.remove()
}

let pollInFlight = false
async function pollStatus() {
  if (!props.orderId || outcome.value || pollInFlight) return
  pollInFlight = true
  try {
    // Stripe 直接查询上游，微信和支付宝当面付在本地 pending 时节流补查，以补偿遗漏的回调。
    const upstreamOutTradeNo = isProcessing.value ? '' : upstreamVerificationOutTradeNo()
    let order = upstreamOutTradeNo && props.paymentType === 'stripe'
      ? await verifyOrderWithUpstream(upstreamOutTradeNo)
      : await paymentStore.pollOrderStatus(props.orderId)
    if (outcome.value) return
    order = await tryRecoverPendingOrder(order)
    if (outcome.value) return
    applyResolvedOrderStatus(order)
  } finally {
    pollInFlight = false
  }
}

function applyResolvedOrderStatus(order: PaymentOrder | null): boolean {
  if (!order) return false
  if (order.status === 'PROCESSING') {
    enterProcessingState()
    return true
  }
  if (isSuccessStatus(order.status)) {
    cleanup()
    paidOrder.value = order
    setOutcome('success')
    emit('success')
    return true
  } else if (order.status === 'CANCELLED') {
    cleanup()
    setOutcome('cancelled')
    return true
  } else if (order.status === 'EXPIRED' || order.status === 'FAILED') {
    cleanup()
    setOutcome('expired')
    return true
  }
  return false
}

function enterProcessingState() {
  isProcessing.value = true
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
  startPolling(PROCESSING_POLL_INTERVAL_MS)
}

async function verifyOrderWithUpstream(outTradeNo: string): Promise<PaymentOrder | null> {
  try {
    const response = await paymentAPI.verifyOrder(outTradeNo)
    return response.data
  } catch (err: unknown) {
    console.warn('[payment] Failed to verify order upstream:', err)
    return paymentStore.pollOrderStatus(props.orderId)
  }
}

async function tryRecoverPendingOrder(order: PaymentOrder | null): Promise<PaymentOrder | null> {
  if (!order || (!isWxpay.value && !isAlipay.value)) return order
  const outTradeNo = String(order.out_trade_no || props.outTradeNo || '').trim()
  if (!outTradeNo) return order
  if (String(order.status || '').trim().toUpperCase() !== 'PENDING') return order
  const now = Date.now()
  if (verifyAttempts >= VERIFY_RETRY_MAX_ATTEMPTS || now - lastVerifyAt < VERIFY_RETRY_INTERVAL_MS) {
    return order
  }

  lastVerifyAt = now
  verifyAttempts += 1
  try {
    const response = await paymentAPI.verifyOrder(outTradeNo)
    return response.data ?? order
  } catch {
    return order
  }
}

function startCountdown(seconds: number) {
  if (isProcessing.value) return
  remainingSeconds.value = Math.max(0, seconds)
  if (remainingSeconds.value <= 0) {
    void pollStatus()
    return
  }
  if (countdownTimer) { clearInterval(countdownTimer); countdownTimer = null }
  countdownTimer = setInterval(() => {
    if (isProcessing.value) return
    remainingSeconds.value--
    if (remainingSeconds.value <= 0) {
      remainingSeconds.value = 0
      if (countdownTimer) clearInterval(countdownTimer)
      countdownTimer = null
      void pollStatus()
    }
  }, 1000)
}

async function handleCancel() {
  if (!props.orderId || cancelling.value) return
  cancelling.value = true
  try {
    const response = await paymentAPI.cancelOrder(props.orderId)
    cleanup()
    if ((response.data as { message?: string } | undefined)?.message === 'already_paid') {
      const upstreamOutTradeNo = upstreamVerificationOutTradeNo()
      const order = upstreamOutTradeNo
        ? await verifyOrderWithUpstream(upstreamOutTradeNo)
        : await paymentStore.pollOrderStatus(props.orderId)
      if (!applyResolvedOrderStatus(order)) {
        startCountdown(remainingSeconds.value)
        startPolling()
      }
    } else {
      setOutcome('cancelled')
    }
  } catch (err: unknown) {
    appStore.showError(extractI18nErrorMessage(err, t, 'payment.errors', t('common.error')))
  } finally {
    cancelling.value = false
  }
}

function handleDone() { cleanup(); emit('done') }

function cleanup() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null }
  if (countdownTimer) { clearInterval(countdownTimer); countdownTimer = null }
  pollIntervalMs = 0
  alipayLauncher?.dispose()
  alipayLauncher = null
}

function startPolling(intervalMs = PENDING_POLL_INTERVAL_MS) {
  if (pollTimer && pollIntervalMs === intervalMs) return
  if (pollTimer) clearInterval(pollTimer)
  pollIntervalMs = intervalMs
  pollTimer = setInterval(pollStatus, intervalMs)
}

// Initialize on mount
qrUrl.value = props.qrCode
isProcessing.value = false
verifyAttempts = 0
lastVerifyAt = 0
let seconds = 30 * 60
if (props.expiresAt) {
  seconds = Math.floor((new Date(props.expiresAt).getTime() - Date.now()) / 1000)
}
startCountdown(seconds)
startPolling()
renderQR()

watch([() => qrUrl.value, showQRCode], () => renderQR())
onMounted(() => {
  if (!isMobileAlipayDeepLink.value) return
  alipayLauncher = createAlipayDeepLinkLauncher({
    qrCode: qrUrl.value,
    document,
    lifecycleTarget: window,
    userAgent: window.navigator.userAgent,
    assignLocation: (url) => window.location.assign(url),
    onStateChange: updateDeepLinkState,
  })
  alipayLauncher.launch()
})
onUnmounted(() => cleanup())
</script>
