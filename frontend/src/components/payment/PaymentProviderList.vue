<template>
  <SettingsCard
    :title="t('admin.settings.payment.providerManagement')"
    :description="t('admin.settings.payment.providerManagementDesc')"
  >
    <template #actions>
      <div class="flex items-center gap-2">
        <button
          type="button"
          @click="emit('refresh')"
          :disabled="loading"
          class="btn btn-secondary btn-icon"
          :title="t('common.refresh')"
          :aria-label="t('common.refresh')"
        >
          <Icon name="refresh" size="sm" :class="loading ? 'animate-spin' : ''" />
        </button>
        <button
          type="button"
          @click="emit('create')"
          :disabled="!canCreate"
          :class="canCreate
            ? 'btn btn-primary btn-sm h-9'
            : 'btn btn-secondary btn-sm h-9 cursor-not-allowed opacity-50'"
        >
          {{ t('admin.settings.payment.createProvider') }}
        </button>
      </div>
    </template>

    <!-- 服务商列表 -->
    <div>
      <!-- Loading -->
      <ContentSkeleton v-if="loading && !providers.length" variant="list" :rows="3" class="py-4" />

      <!-- Provider cards (draggable) -->
      <VueDraggable
        v-if="providers.length"
        v-model="localProviders"
        :animation="200"
        handle=".drag-handle"
        class="space-y-3"
        @end="onDragEnd"
      >
        <div v-for="p in localProviders" :key="p.id" class="flex items-start gap-2">
          <div class="drag-handle mt-3 flex cursor-grab items-center text-gray-300 hover:text-gray-500 active:cursor-grabbing dark:text-dark-600 dark:hover:text-dark-400">
            <Icon name="grip" size="md" class="h-5 w-5" />
          </div>
          <div class="min-w-0 flex-1">
            <ProviderCard
              :provider="p"
              :enabled="isEnabled(p.provider_key)"
              :available-types="getTypes(p.provider_key)"
              :updating="isUpdating(p.id)"
              @toggle-field="(field) => emit('toggleField', p, field)"
              @toggle-type="(type) => emit('toggleType', p, type)"
              @edit="emit('edit', p)"
              @delete="emit('delete', p)"
            />
          </div>
        </div>
      </VueDraggable>

      <!-- Empty -->
      <div v-else-if="!loading" class="py-6 text-center">
        <p class="text-sm text-gray-500 dark:text-gray-400">
          {{ canCreate
            ? t('admin.settings.payment.noProviders')
            : t('admin.settings.payment.enableTypesFirst') }}
        </p>
        <button
          type="button"
          v-if="canCreate"
          @click="emit('create')"
          class="btn btn-primary btn-sm h-9 mt-2"
        >
          {{ t('admin.settings.payment.createProvider') }}
        </button>
      </div>
    </div>
  </SettingsCard>
</template>

<script setup lang="ts">
import ContentSkeleton from '@/components/common/ContentSkeleton.vue'
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { VueDraggable } from 'vue-draggable-plus'
import Icon from '@/components/icons/Icon.vue'
import SettingsCard from '@/components/common/settings/SettingsCard.vue'
import ProviderCard from './ProviderCard.vue'
import type { ProviderInstance } from '@/types/payment'
import type { TypeOption } from './providerConfig'
import { getAvailableTypes } from './providerConfig'

const props = defineProps<{
  providers: ProviderInstance[]
  loading: boolean
  canCreate: boolean
  enabledPaymentTypes: string[]
  allPaymentTypes: TypeOption[]
  redirectLabel: string
  updatingProviderIds?: number[]
}>()

const emit = defineEmits<{
  refresh: []
  create: []
  edit: [provider: ProviderInstance]
  delete: [provider: ProviderInstance]
  toggleField: [provider: ProviderInstance, field: 'enabled' | 'refund_enabled' | 'allow_user_refund']
  toggleType: [provider: ProviderInstance, type: string]
  reorder: [providers: { id: number; sort_order: number }[]]
}>()

const { t } = useI18n()

const localProviders = ref<ProviderInstance[]>([])

watch(() => props.providers, (val) => {
  localProviders.value = val.map(normalizeProvider)
}, { immediate: true })

function normalizeProvider(provider: ProviderInstance): ProviderInstance {
  return {
    ...provider,
    // 将接口返回的 null supported_types 转为空数组后渲染。
    supported_types: Array.isArray(provider.supported_types) ? provider.supported_types : [],
  }
}

function onDragEnd() {
  const updates = localProviders.value.map((p, idx) => ({
    id: p.id,
    sort_order: idx,
  }))
  emit('reorder', updates)
}

function isEnabled(providerKey: string): boolean {
  return props.enabledPaymentTypes.includes(providerKey)
}

function isUpdating(providerId: number): boolean {
  // 父组件按服务商 ID 标记更新中，列表只负责把禁用态传给具体卡片。
  return props.updatingProviderIds?.includes(providerId) ?? false
}

function getTypes(providerKey: string): TypeOption[] {
  return getAvailableTypes(providerKey, props.allPaymentTypes, props.redirectLabel)
    .map(opt => opt.label === opt.value
      ? { ...opt, label: t(`payment.methods.${opt.value}`, opt.value) }
      : opt,
    )
}
</script>
