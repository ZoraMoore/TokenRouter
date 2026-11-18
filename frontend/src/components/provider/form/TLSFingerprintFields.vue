<template>
  <SettingsSection>
    <SettingToggleRow
      v-if="!hideToggle"
      :id="`${uid}-enabled`"
      v-model="enabled"
      :label="t('admin.providers.quotaControl.tlsFingerprint.label')"
      :hint="t('admin.providers.quotaControl.tlsFingerprint.hint')"
      :testid="testIdPrefix && `${testIdPrefix}-toggle`"
    />
    <Collapse :open="hideToggle || enabled" unmount-on-hide>
      <SettingsSubpanel>
        <div>
          <label :for="`${uid}-profile`" class="input-label">{{ t('admin.providers.quotaControl.tlsFingerprint.profile') }}</label>
          <Select
            :id="`${uid}-profile`"
            v-model="profileId"
            :data-testid="testIdPrefix && `${testIdPrefix}-profile`"
            :options="profileOptions"
          />
        </div>
        <div v-if="routerOptions">
          <label :for="`${uid}-router`" class="input-label">{{ t('admin.providers.quotaControl.tlsFingerprint.router') }}</label>
          <Select
            :id="`${uid}-router`"
            v-model="routerId"
            :data-testid="testIdPrefix && `${testIdPrefix}-router`"
            :options="routerOptions"
          />
          <p class="input-hint">{{ t('admin.providers.quotaControl.tlsFingerprint.routerHint') }}</p>
        </div>
      </SettingsSubpanel>
    </Collapse>
  </SettingsSection>
</template>

<script setup lang="ts">
import { useId } from 'vue'
import { useI18n } from 'vue-i18n'
import Collapse from '@/components/common/Collapse.vue'
import Select, { type SelectOption } from '@/components/common/Select.vue'
import SettingsSection from '@/components/common/settings/SettingsSection.vue'
import SettingsSubpanel from '@/components/common/settings/SettingsSubpanel.vue'
import SettingToggleRow from '@/components/common/settings/SettingToggleRow.vue'

// TLS 指纹伪装：Qoder COSY、Anthropic OAuth 与 OpenAI OAuth 共用，只有 OpenAI 提供路由选择。
defineProps<{
  profileOptions: SelectOption[]
  /** 传入时显示路由选择。 */
  routerOptions?: SelectOption[]
  testIdPrefix?: string
  /** 批量编辑由外层应用开关控制显示。 */
  hideToggle?: boolean
}>()
const enabled = defineModel<boolean>('enabled', { default: false })
const profileId = defineModel<number | null>('profileId', { required: true })
const routerId = defineModel<number | null>('routerId', { default: null })

const { t } = useI18n()
const uid = useId()
</script>
