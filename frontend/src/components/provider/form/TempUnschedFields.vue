<template>
  <SettingsSection>
    <SettingToggleRow
      :id="`${uid}-enabled`"
      v-model="enabled"
      :label="t('admin.providers.tempUnschedulable.title')"
      :hint="t('admin.providers.tempUnschedulable.hint')"
      testid="temp-unsched-toggle"
    />
    <Collapse :open="enabled" unmount-on-hide>
      <TempUnschedRulesEditor v-model="rules" data-provider-field="temp-unsched" />
    </Collapse>
  </SettingsSection>
</template>

<script setup lang="ts">
import { useId } from 'vue'
import { useI18n } from 'vue-i18n'
import Collapse from '@/components/common/Collapse.vue'
import SettingsSection from '@/components/common/settings/SettingsSection.vue'
import SettingToggleRow from '@/components/common/settings/SettingToggleRow.vue'
import TempUnschedRulesEditor, { type TempUnschedRuleForm } from '../TempUnschedRulesEditor.vue'

// 临时不可调度规则的开关与规则列表，创建与编辑共用。
const enabled = defineModel<boolean>('enabled', { required: true })
const rules = defineModel<TempUnschedRuleForm[]>('rules', { required: true })

const { t } = useI18n()
const uid = useId()
</script>
