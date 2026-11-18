<template>
  <div class="space-y-4">
    <!-- 快捷金额：分段标题由父组件提供，这里只保留按钮网格。 -->
    <div class="grid grid-cols-3 gap-2 sm:grid-cols-5">
      <button
        v-for="amt in filteredAmounts"
        :key="amt"
        type="button"
        :aria-pressed="modelValue === amt"
        :class="[
          'h-9 rounded-control border px-3 text-center text-sm font-medium tabular-nums transition-colors',
          modelValue === amt
            ? 'border-primary-500 bg-primary-500/8 text-primary-600 ring-1 ring-primary-500 dark:text-primary-400'
            : 'border-gray-200 bg-white text-gray-700 hover:border-gray-300 dark:border-dark-600 dark:bg-dark-950 dark:text-dark-100 dark:hover:border-dark-500',
        ]"
        @click="selectAmount(amt)"
      >
        {{ amt }}
      </button>
    </div>

    <!-- 自定义金额 -->
    <div>
      <label for="payment-custom-amount" class="mb-1.5 block text-xs font-medium text-gray-500 dark:text-dark-400">
        {{ t('payment.customAmount') }}
      </label>
      <div class="input-icon-wrap">
        <span class="input-icon text-gray-400 dark:text-dark-500">
          {{ symbol }}
        </span>
        <input
          id="payment-custom-amount"
          type="text"
          inputmode="decimal"
          :value="customText"
          :placeholder="placeholderText"
          :class="['input input-has-icon tabular-nums', symbol.length === 1 ? 'input-icon-text' : '']"
          @input="handleInput"
        />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, computed, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { currencySymbol } from './currency'

const props = withDefaults(defineProps<{
  amounts?: number[]
  modelValue: number | null
  min?: number
  max?: number
  /** 支付币种，决定自定义金额输入框的货币符号前缀。 */
  currency?: string
}>(), {
  amounts: () => [10, 20, 50, 100, 200, 500, 1000, 2000, 5000],
  min: 0,
  max: 0,
  currency: '',
})

const emit = defineEmits<{
  'update:modelValue': [value: number | null]
}>()

const { t } = useI18n()

const customText = ref('')

// 金额前缀使用支付币种符号，NZ$、KWD 等多字符符号使用默认留白。
const symbol = computed(() => currencySymbol(props.currency))

// 0 = no limit
const filteredAmounts = computed(() =>
  props.amounts.filter((a) => (props.min <= 0 || a >= props.min) && (props.max <= 0 || a <= props.max))
)

const placeholderText = computed(() => {
  if (props.min > 0 && props.max > 0) return `${props.min} - ${props.max}`
  if (props.min > 0) return `≥ ${props.min}`
  if (props.max > 0) return `≤ ${props.max}`
  return t('payment.enterAmount')
})

const AMOUNT_PATTERN = /^\d*(\.\d{0,2})?$/

function selectAmount(amt: number) {
  customText.value = String(amt)
  emit('update:modelValue', amt)
}

function handleInput(e: Event) {
  const val = (e.target as HTMLInputElement).value
  if (!AMOUNT_PATTERN.test(val)) return
  customText.value = val
  if (val === '') {
    emit('update:modelValue', null)
    return
  }
  const num = parseFloat(val)
  if (!isNaN(num) && num > 0) {
    emit('update:modelValue', num)
  } else {
    emit('update:modelValue', null)
  }
}

watch(() => props.modelValue, (v) => {
  if (v !== null && String(v) !== customText.value) {
    customText.value = String(v)
  }
}, { immediate: true })
</script>
