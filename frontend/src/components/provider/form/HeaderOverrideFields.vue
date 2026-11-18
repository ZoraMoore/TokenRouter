<template>
  <SettingsSection>
    <SettingToggleRow
      :id="`${uid}-enabled`"
      v-model="enabled"
      :label="t('admin.providers.headerOverride.title')"
      :hint="t('admin.providers.headerOverride.hint')"
      :testid="testid"
    />
    <Collapse :open="enabled" unmount-on-hide>
      <SettingsSubpanel>
        <SettingsNotice>
          <p>{{ t('admin.providers.headerOverride.info') }}</p>
          <slot name="notice" />
        </SettingsNotice>
        <HeaderOverrideEditor :rows="rows" @update:rows="rows = $event" />
      </SettingsSubpanel>
    </Collapse>
    <slot name="disabled" v-if="!enabled" />
  </SettingsSection>
</template>

<script setup lang="ts">
import { useId } from 'vue'
import { useI18n } from 'vue-i18n'
import Collapse from '@/components/common/Collapse.vue'
import SettingsNotice from '@/components/common/settings/SettingsNotice.vue'
import SettingsSection from '@/components/common/settings/SettingsSection.vue'
import SettingsSubpanel from '@/components/common/settings/SettingsSubpanel.vue'
import SettingToggleRow from '@/components/common/settings/SettingToggleRow.vue'
import type { HeaderOverrideRow } from '../credentialsBuilder'
import HeaderOverrideEditor from '../HeaderOverrideEditor.vue'

// 请求头覆写的开关和规则列表；批量编辑通过插槽补充替换与关闭说明。
defineProps<{ testid?: string }>()
const enabled = defineModel<boolean>('enabled', { required: true })
const rows = defineModel<HeaderOverrideRow[]>('rows', { required: true })

const { t } = useI18n()
const uid = useId()
</script>
