<template>
  <button
    v-if="queryEnabled"
    type="button"
    class="inline-flex items-center gap-0.5 rounded-compact px-1.5 py-0.5 text-xs font-medium text-primary-600 transition-colors hover:bg-primary-50 disabled:cursor-not-allowed disabled:opacity-50 dark:text-primary-500 dark:hover:bg-primary-500/8"
    :disabled="loading"
    :title="t('admin.providers.upstreamUsage.query')"
    :aria-label="t('admin.providers.upstreamUsage.query')"
    @click="query"
  >
    <Icon
      name="refresh"
      size="xs"
      :class="{ 'animate-spin': loading }"
      :stroke-width="2"
    />
    {{ t('admin.providers.usageWindow.activeQuery') }}
  </button>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import type { Provider } from '@/types'
import Icon from '@/components/icons/Icon.vue'
import { isUpstreamUsageQueryEnabled } from '@/utils/upstreamUsage'

const props = withDefaults(defineProps<{
  provider: Provider
  loading?: boolean
  request?: ((provider: Provider, options?: { force?: boolean }) => void) | null
}>(), {
  loading: false,
  request: null
})

const { t } = useI18n()

// 查询按钮只负责管理员显式操作；配置关闭时由内容组件显示关闭状态。
const queryEnabled = computed(() => isUpstreamUsageQueryEnabled(props.provider))

const query = () => {
  props.request?.(props.provider, { force: true })
}
</script>
