<template>
  <!-- 协议标签放在深底浮层里，配色和属性浮层的能力标签一致；完整端点在悬停提示里。 -->
  <div v-if="items.length" data-testid="model-protocols">
    <div class="flex items-center justify-between gap-4">
      <p class="text-gray-400 dark:text-dark-400">{{ t('marketplace.protocols') }}</p>
      <!-- 图例的色块和原生协议标签同色，只有出现原生协议时显示。 -->
      <span
        v-if="hasNative"
        class="inline-flex items-center gap-1.5 text-gray-400 dark:text-dark-400"
        data-testid="model-protocols-native-legend"
      >
        <span class="h-2.5 w-2.5 rounded-full bg-emerald-500/20 ring-1 ring-inset ring-emerald-400/70"></span>
        {{ t('marketplace.protocolNative') }}
      </span>
    </div>
    <ul class="mt-2 flex flex-wrap gap-1.5">
      <!-- 原生协议用绿色描边和淡绿底标出，经过转换的协议保持中性底色。 -->
      <li
        v-for="protocol in items"
        :key="protocol.id"
        class="inline-flex items-center gap-1 rounded-compact px-1.5 py-0.5 text-white dark:text-dark-100"
        :class="protocol.native
          ? 'bg-emerald-500/15 ring-1 ring-inset ring-emerald-400/60'
          : 'bg-white/5 dark:bg-dark-800'"
        :data-protocol="protocol.id"
        :data-native="protocol.native"
        :title="`${protocol.endpoint} · ${t(protocol.native ? 'marketplace.protocolNativeHint' : 'marketplace.protocolConvertedHint')}`"
      >
        <!-- OpenAI 图标在浅色主题下默认是黑色，这里传 currentColor，让它在深底浮层里跟随白色文字。 -->
        <ProviderIcon
          :brand="protocol.brand"
          size="14px"
          :color="protocol.brand === 'openai' ? 'currentColor' : ''"
          class="select-none"
        />
        <span class="whitespace-nowrap">{{ t(`marketplace.protocolNames.${protocol.id}`) }}</span>
      </li>
    </ul>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import ProviderIcon from '@/components/common/ProviderIcon.vue'
import { marketplaceProtocols } from '@/utils/marketplaceProtocols'
import type { ProtocolID } from '@/types'

const props = defineProps<{
  protocols?: ProtocolID[]
  // nativeProtocols 是 protocols 里不经过协议转换的那部分。
  nativeProtocols?: ProtocolID[]
}>()
const { t } = useI18n()

const items = computed(() => {
  const native = new Set(props.nativeProtocols ?? [])
  return marketplaceProtocols(props.protocols).map(protocol => ({ ...protocol, native: native.has(protocol.id) }))
})
const hasNative = computed(() => items.value.some(protocol => protocol.native))
</script>
