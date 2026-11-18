<template>
  <div v-if="visible" class="space-y-1">
    <div class="flex flex-wrap items-center gap-1.5">
      <button
        type="button"
        class="inline-flex items-center gap-0.5 rounded-compact px-1.5 py-0.5 text-xs font-medium text-primary-600 transition-colors hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-500 dark:hover:bg-primary-500/8"
        :disabled="loading"
        :title="t('admin.providers.usageWindow.grokProbeTooltip')"
        @click="handleProbe"
      >
        <Icon
          name="refresh"
          size="md"
          :animate-on-hover="false"
          class="h-2.5 w-2.5"
          :class="{ 'animate-spin': loading }"
        />
        {{ t('admin.providers.usageWindow.grokProbe') }}
      </button>
    </div>

    <!-- 紧凑模式下父组件已显示 7 天、30 天、预付余额或 24 小时信息，此处只展示错误。 -->
    <div
      v-if="!compact && summary"
      class="text-xs text-gray-600 dark:text-gray-300"
    >
      {{ summary }}
    </div>
    <div v-if="error" class="truncate text-xs text-red-600 dark:text-red-400" :title="error">
      {{ truncatedError }}
    </div>
  </div>
</template>

<script setup lang="ts">
import Icon from '@/components/icons/Icon.vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { GrokQuotaProbeResult } from '@/api/admin/grok'
import type { Provider } from '@/types'

const props = withDefaults(
  defineProps<{
    provider: Provider
    /** 为 true 时显示探测按钮与错误，周度摘要由外层展示。 */
    compact?: boolean
  }>(),
  { compact: false }
)

const emit = defineEmits<{ probed: [result: GrokQuotaProbeResult] }>()

const { t } = useI18n()

const visible = computed(() => props.provider.platform === 'grok' && props.provider.type === 'oauth')
const loading = ref(false)
const error = ref<string | null>(null)
const data = ref<GrokQuotaProbeResult | null>(null)

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

const summary = computed(() => {
  if (props.compact || !data.value) return ''
  // 非紧凑模式的回退展示，存在周度百分比时显示简要信息。
  const billing = data.value.billing
  if (billing?.period_type?.toLowerCase() === 'weekly' && billing.usage_percent != null) {
    return t('admin.providers.usageWindow.grokWeeklyUsage', {
      percent: Math.round(Math.min(100, Math.max(0, billing.usage_percent)))
    })
  }
  return ''
})

const truncatedError = computed(() => {
  if (!error.value) return ''
  return error.value.length > 80 ? `${error.value.slice(0, 80)}...` : error.value
})

const handleProbe = async () => {
  if (loading.value) return
  loading.value = true
  error.value = null
  try {
    data.value = await adminAPI.grok.queryQuota(props.provider.id)
    error.value = data.value.probe_error || null
    emit('probed', data.value)
  } catch (e) {
    error.value = extractErrorMessage(e)
  } finally {
    loading.value = false
  }
}

watch(
  () => props.provider.id,
  () => {
    data.value = null
    error.value = null
    loading.value = false
  }
)
</script>
