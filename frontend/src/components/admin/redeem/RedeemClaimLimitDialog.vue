<template>
  <BaseDialog
    :show="true"
    :title="t('admin.redeem.claimLimit.title')"
    :close-on-escape="!saving"
    @close="close"
  >
    <form v-if="settings" id="redeem-claim-limit-form" class="space-y-4" novalidate @submit.prevent="save">
      <SettingToggleRow
        id="redeem-claim-limit-enabled"
        v-model="form.enabled"
        :label="t('admin.redeem.claimLimit.enabled')"
        :hint="t('admin.redeem.claimLimit.hint')"
        :disabled="saving"
        testid="claim-limit-enabled"
      />
      <div class="grid gap-3 sm:grid-cols-2">
        <div class="min-w-0">
          <label for="redeem-claim-limit-starts-at" class="input-label">{{ t('admin.redeem.claimLimit.startsAt') }}</label>
          <input
            id="redeem-claim-limit-starts-at"
            v-model="form.startsAt"
            type="datetime-local"
            step="1"
            class="input min-w-0"
            :disabled="saving"
            :required="form.enabled"
          />
        </div>
        <div class="min-w-0">
          <label for="redeem-claim-limit-ends-at" class="input-label">{{ t('admin.redeem.claimLimit.endsAt') }}</label>
          <input
            id="redeem-claim-limit-ends-at"
            v-model="form.endsAt"
            type="datetime-local"
            step="1"
            class="input min-w-0"
            :disabled="saving"
            :required="form.enabled"
          />
        </div>
      </div>
      <p class="input-hint">{{ t('admin.redeem.claimLimit.timeZone', { timeZone }) }}</p>
      <SettingsNotice v-if="startsBeforeTracking" tone="info">
        {{ t('admin.redeem.claimLimit.trackingSince', { time: formatDateTime(new Date(settings.tracking_since * 1000)) }) }}
      </SettingsNotice>
      <SettingsNotice v-if="error" tone="error">{{ error }}</SettingsNotice>
    </form>
    <p v-else-if="loading" role="status" class="text-sm text-gray-600 dark:text-gray-300">
      {{ t('common.loading') }}
    </p>
    <SettingsNotice v-else tone="error">
      <p>{{ t('admin.redeem.claimLimit.loadFailed') }}</p>
      <button type="button" class="btn btn-secondary btn-sm" @click="load">{{ t('common.retry') }}</button>
    </SettingsNotice>

    <template #footer>
      <div class="flex justify-end gap-3">
        <button type="button" class="btn btn-secondary" :disabled="saving" @click="close">
          {{ t('common.cancel') }}
        </button>
        <button type="submit" form="redeem-claim-limit-form" class="btn btn-primary" :disabled="!settings || saving">
          {{ saving ? t('common.saving') : t('common.save') }}
        </button>
      </div>
    </template>
  </BaseDialog>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { adminAPI } from '@/api/admin'
import type { RedeemClaimSettings } from '@/api/admin/redeem'
import BaseDialog from '@/components/common/BaseDialog.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import SettingToggleRow from '@/components/common/settings/SettingToggleRow.vue'
import { useAppStore } from '@/stores/app'
import { formatDateTime, formatDateTimeLocalInput, parseDateTimeLocalInput } from '@/utils/format'

const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()
const appStore = useAppStore()
const abort = new AbortController()
const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone
const settings = ref<RedeemClaimSettings | null>(null)
const form = reactive({ enabled: false, startsAt: '', endsAt: '' })
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const startsBeforeTracking = computed(() => {
  const startsAt = parseDateTimeLocalInput(form.startsAt)
  return form.enabled && startsAt !== null && startsAt < (settings.value?.tracking_since ?? 0)
})

// 兼容通过 API 保存的秒级时段，重新打开后可按原精度保存。
function formatClaimTime(value: number | null) {
  const minute = formatDateTimeLocalInput(value)
  if (!minute || value === null) return minute
  const second = new Date(value * 1000).getSeconds()
  return second ? `${minute}:${String(second).padStart(2, '0')}` : minute
}

async function load() {
  if (loading.value) return
  loading.value = true
  try {
    const saved = await adminAPI.redeem.getClaimLimit(abort.signal)
    if (abort.signal.aborted) return
    settings.value = saved
    form.enabled = saved.enabled
    form.startsAt = formatClaimTime(saved.starts_at)
    form.endsAt = formatClaimTime(saved.ends_at)
  } catch {
    settings.value = null
  } finally {
    loading.value = false
  }
}

function close() {
  if (!saving.value) emit('close')
}

async function save() {
  if (!settings.value || saving.value) return
  const startsAt = parseDateTimeLocalInput(form.startsAt)
  const endsAt = parseDateTimeLocalInput(form.endsAt)
  const needsWindow = form.enabled || Boolean(form.startsAt || form.endsAt)
  if (needsWindow && (startsAt === null || endsAt === null || startsAt >= endsAt)) {
    error.value = t('admin.redeem.claimLimit.invalid')
    return
  }
  error.value = ''
  saving.value = true
  try {
    await adminAPI.redeem.setClaimLimit({ enabled: form.enabled, starts_at: startsAt, ends_at: endsAt })
    if (abort.signal.aborted) return
    appStore.showSuccess(t('admin.redeem.claimLimit.saved'))
    emit('close')
  } catch {
    error.value = t('admin.redeem.claimLimit.saveFailed')
  } finally {
    saving.value = false
  }
}

onMounted(load)
onUnmounted(() => abort.abort())
</script>
