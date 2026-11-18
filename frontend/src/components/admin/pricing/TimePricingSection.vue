<template>
  <RuleListEditor
    class="mt-3 border-t border-gray-200 pt-3 dark:border-dark-600"
    :items="modelValue.periods"
    :title="t('admin.pricing.form.timePricing')"
    :add-label="t('admin.pricing.form.addTimePeriod')"
    :remove-label="t('admin.pricing.form.removeTimePeriod')"
    variant="card"
    :item-label="(index) => t('common.ruleIndex', { index: index + 1 })"
    test-id="time-periods"
    @add="addPeriod"
    @remove="removePeriod"
  >
    <template #header-extra>
      <div class="grid grid-cols-1 gap-2 sm:grid-cols-2">
        <div class="min-w-0">
          <label class="block text-xs text-gray-400">
            {{ t('admin.pricing.form.timezone') }}
          </label>
          <Select
            :model-value="modelValue.timezone"
            :options="timezoneOptions"
            :aria-label="t('admin.pricing.form.timezone')"
            data-testid="time-pricing-timezone"
            searchable
            creatable
            class="mt-1 w-full"
            @update:model-value="updateTimezone"
          />
        </div>
        <div class="min-w-0">
          <label class="block text-xs text-gray-400">
            {{ t('admin.pricing.form.timePricingDayScope') }}
          </label>
          <Select
            :model-value="modelValue.weekdays_only"
            :options="dayScopeOptions"
            :aria-label="t('admin.pricing.form.timePricingDayScope')"
            data-testid="time-pricing-day-scope"
            class="mt-1 w-full"
            @update:model-value="updateDayScope"
          />
        </div>
      </div>
    </template>
    <template #row="{ item: period, index }">
      <div class="grid grid-cols-1 gap-2 sm:grid-cols-3">
        <div class="min-w-0">
          <label :for="`${inputIdPrefix}-start-${index}`" class="block text-xs text-gray-400">
            {{ t('admin.pricing.form.startTime') }}
          </label>
          <input
            :id="`${inputIdPrefix}-start-${index}`"
            :value="period.start_time"
            type="text"
            inputmode="numeric"
            maxlength="8"
            placeholder="HH:mm:ss"
            pattern="[0-9]{2}:[0-9]{2}:[0-9]{2}"
            autocomplete="off"
            class="input mt-1 w-full text-sm"
            @input="updatePeriod(index, 'start_time', normalizeClockTime(($event.target as HTMLInputElement).value))"
          />
        </div>
        <div class="min-w-0">
          <label :for="`${inputIdPrefix}-end-${index}`" class="block text-xs text-gray-400">
            {{ t('admin.pricing.form.endTime') }}
          </label>
          <input
            :id="`${inputIdPrefix}-end-${index}`"
            :value="period.end_time"
            type="text"
            inputmode="numeric"
            maxlength="8"
            placeholder="HH:mm:ss"
            pattern="[0-9]{2}:[0-9]{2}:[0-9]{2}"
            autocomplete="off"
            class="input mt-1 w-full text-sm"
            @input="updatePeriod(index, 'end_time', normalizeClockTime(($event.target as HTMLInputElement).value))"
          />
        </div>
        <div class="min-w-0">
          <label :for="`${inputIdPrefix}-multiplier-${index}`" class="block text-xs text-gray-400">
            {{ t('admin.pricing.form.multiplier') }}
          </label>
          <input
            :id="`${inputIdPrefix}-multiplier-${index}`"
            :value="period.multiplier"
            type="number"
            min="0.01"
            step="0.01"
            class="input mt-1 w-full text-sm"
            @input="updatePeriod(index, 'multiplier', ($event.target as HTMLInputElement).value)"
            @blur="formatMultiplier(index, ($event.target as HTMLInputElement).value)"
          />
        </div>
      </div>
    </template>
  </RuleListEditor>
</template>

<script setup lang="ts">
import { computed, getCurrentInstance } from 'vue'
import { useI18n } from 'vue-i18n'
import RuleListEditor from '@/components/common/RuleListEditor.vue'
import Select from '@/components/common/Select.vue'
import {
  COMMON_TIMEZONES,
  formatTimezoneOffset,
  isValidTimePricingMultiplier,
  type TimePricingFormEntry,
  type TimePricingPeriodFormEntry,
} from './types'

const { t } = useI18n()

const props = defineProps<{ modelValue: TimePricingFormEntry }>()
const emit = defineEmits<{ 'update:modelValue': [value: TimePricingFormEntry] }>()
const inputIdPrefix = `time-pricing-${getCurrentInstance()?.uid}`

const timezoneOptions = COMMON_TIMEZONES.map(value => {
  const offset = formatTimezoneOffset(value)
  return { value, label: offset ? `${value} (${offset})` : value }
})

const dayScopeOptions = computed(() => [
  { value: false, label: t('admin.pricing.form.timePricingEveryDay') },
  { value: true, label: t('admin.pricing.form.timePricingWeekdaysOnly') },
])

function updateTimezone(value: string | number | boolean | null) {
  emit('update:modelValue', { ...props.modelValue, timezone: String(value ?? '') })
}

function updateDayScope(value: string | number | boolean | null) {
  emit('update:modelValue', { ...props.modelValue, weekdays_only: value === true })
}

function normalizeClockTime(value: string): string {
  const normalized = value.replace(/：/g, ':')
  if (normalized === '24:00:00') return '00:00:00'
  return /^\d{2}:\d{2}$/.test(normalized) ? `${normalized}:00` : normalized
}

function addPeriod() {
  emit('update:modelValue', {
    ...props.modelValue,
    periods: [...props.modelValue.periods, { start_time: '', end_time: '', multiplier: '1.00' }],
  })
}

function updatePeriod(index: number, field: keyof TimePricingPeriodFormEntry, value: string) {
  // 保留时间段对象，连续输入和格式化不会让焦点离开当前行。
  const periods = [...props.modelValue.periods]
  periods[index][field] = value
  emit('update:modelValue', { ...props.modelValue, periods })
}

function formatMultiplier(index: number, value: string) {
  if (!isValidTimePricingMultiplier(value)) return
  updatePeriod(index, 'multiplier', Number(value).toFixed(2))
}

function removePeriod(index: number) {
  emit('update:modelValue', {
    ...props.modelValue,
    periods: props.modelValue.periods.filter((_period, current) => current !== index),
  })
}
</script>
